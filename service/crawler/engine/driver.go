package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"sg.scout/service/crawler/urlutil"
)

// fetchFn is the per-page fetch used by the BFS driver (goquery direct fetch
// or crawl4ai sync /crawl). Returns a page-level result; transport-level fatal
// errors (ctx cancel) surface as error.
type fetchFn func(ctx context.Context, rawURL string) (*PageResult, error)

// localJob is one in-process BFS crawl job (research.md §2). All state is
// guarded by mu; Poll drains the page buffer (mirrors cloud paging).
type localJob struct {
	id             string
	status         string // running | completed | cancelled | failed
	err            string
	delay          time.Duration
	limit          int
	fetch          fetchFn
	robots         *robotsCache
	cancel         context.CancelFunc
	mu             sync.Mutex
	completedPages []*PageResult // drained by PollCrawl
	failedPages    []*PageResult // cumulative (FetchErrors)
	visitedCount   int
	doneCh         chan struct{}
}

// localJobs is the in-process job registry shared by local engines
// (goquery + crawl4ai). Job ids are local and never survive a restart.
type localJobs struct {
	mu   sync.Mutex
	jobs map[string]*localJob
	seq  uint64
}

func newLocalJobs() *localJobs {
	return &localJobs{jobs: map[string]*localJob{}}
}

func (j *localJobs) nextID() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.seq++
	return fmt.Sprintf("local-%d", j.seq)
}

// crawlRequest is the driver's own parameter view built from CrawlRequest.
type crawlRequest struct {
	entryURL        string
	maxDepth        int
	allowSubdomains bool
	allowHosts      map[string]bool // normalized external hosts allowed past the same-site boundary
	ignoreRobots    bool            // true = skip robots.txt checks
	includeURLs     []string        // lowercased URL substrings; only discovered links containing one are followed
	limit           int
	delay           time.Duration
}

func newCrawlRequest(req *CrawlRequest) crawlRequest {
	cr := crawlRequest{
		entryURL:        req.URL,
		maxDepth:        req.MaxDiscoveryDepth,
		allowSubdomains: req.AllowSubdomains,
		allowHosts:      map[string]bool{},
		ignoreRobots:    req.IgnoreRobots,
		includeURLs:     req.IncludeURLs,
		limit:           req.Limit,
	}
	for _, h := range req.AllowHosts {
		cr.allowHosts[urlutil.NormalizeHostName(h)] = true
	}
	if cr.limit < 1 {
		cr.limit = 10
	}
	if req.DelaySeconds > 0 {
		cr.delay = time.Duration(req.DelaySeconds) * time.Second
	}
	return cr
}

// submit launches a BFS crawl goroutine and registers the job.
func (j *localJobs) submit(parent context.Context, req *CrawlRequest, fetch fetchFn, robots *robotsCache) (string, error) {
	id := j.nextID()
	ctx, cancel := context.WithCancel(parent)
	job := &localJob{
		id: id, status: "running", fetch: fetch, robots: robots,
		cancel: cancel, doneCh: make(chan struct{}),
	}
	if robots == nil {
		job.robots = newRobotsCache()
	}
	cr := newCrawlRequest(req)
	job.delay, job.limit = cr.delay, cr.limit
	j.mu.Lock()
	j.jobs[id] = job
	j.mu.Unlock()
	go job.run(ctx, cr)
	return id, nil
}

// get loads a registered job.
func (j *localJobs) get(id string) (*localJob, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	job, ok := j.jobs[id]
	if !ok {
		return nil, fmt.Errorf("本地抓取任务 %s 不存在（服务可能已重启，本地任务不跨重启保留）", id)
	}
	return job, nil
}

// enqueueItem is a BFS queue entry. external marks a link that crossed the
// same-site boundary via the allow_hosts whitelist; external pages are fetched
// once and never expanded (leaf), keeping whitelisted hosts bounded.
type enqueueItem struct {
	rawURL   string
	depth    int
	external bool
}

// run executes the BFS loop: layered, same-host bounded, deduped, throttled,
// page-limit capped, cancellable. Robots is enforced for every URL.
func (job *localJob) run(ctx context.Context, cr crawlRequest) {
	defer close(job.doneCh)
	visited := map[string]bool{} // url_key -> enqueued
	scopeHost := ""              // canonical host of the entry's final URL
	queue := []enqueueItem{{rawURL: cr.entryURL, depth: 0}}

	okCount, failCount := 0, 0
	for len(queue) > 0 {
		if ctx.Err() != nil {
			job.finish("cancelled", ctx.Err().Error())
			return
		}
		if job.limit > 0 && okCount+failCount >= job.limit {
			job.finish("completed", "")
			return
		}
		item := queue[0]
		queue = queue[1:]

		key, err := urlutil.URLKey(item.rawURL)
		if err != nil || visited[key] {
			continue
		}
		visited[key] = true
		job.visitedCount++

		u, perr := url.Parse(item.rawURL)
		if perr != nil {
			job.addFailed(item.rawURL, "URL 解析失败", item.depth)
			failCount++
			continue
		}
		// Same-site boundary (FR-011/016): scope is the entry's final host.
		// allow_hosts whitelist lets named external hosts through once (leaf).
		external := item.external
		if scopeHost == "" {
			scopeHost = urlutil.NormalizeHostName(u.Host)
		} else if !job.inScope(scopeHost, u.Host, cr.allowSubdomains) {
			if !cr.allowHosts[urlutil.NormalizeHostName(u.Host)] {
				continue // cross-site links are never fetched (not a failure)
			}
			external = true // whitelisted external page: fetch once, never expand
		}
		if !cr.ignoreRobots && !job.robots.allowed(robotsClient, u) {
			job.addFailed(item.rawURL, "robots.txt 禁止抓取", item.depth)
			failCount++
			continue
		}

		// Polite pacing between requests (FR-018 throttle).
		if (okCount+failCount) > 0 && job.delay > 0 {
			select {
			case <-ctx.Done():
				job.finish("cancelled", ctx.Err().Error())
				return
			case <-time.After(job.delay):
			}
		}

		pr, ferr := job.fetch(ctx, item.rawURL)
		if ferr != nil {
			if errors.Is(ferr, context.Canceled) || ctx.Err() != nil {
				job.finish("cancelled", ctx.Err().Error())
				return
			}
			job.addFailed(item.rawURL, ferr.Error(), item.depth)
			failCount++
			continue
		}
		if pr.Failed {
			job.addFailed(pr.URL, pr.Err, item.depth)
			failCount++
			continue
		}
		// Success: a redirect may move a page onto a whitelisted external host —
		// treat it like any whitelisted page: record success, do not expand.
		if finalHost, e2 := url.Parse(pr.URL); e2 == nil && finalHost.Host != "" {
			norm := urlutil.NormalizeHostName(finalHost.Host)
			if scopeHost == "" {
				scopeHost = norm
			} else if norm != scopeHost && !job.inScope(scopeHost, finalHost.Host, cr.allowSubdomains) {
				if !cr.allowHosts[norm] {
					// Redirect escaped the scope — record as failure, do not expand.
					job.addFailed(pr.URL, "重定向到站外: "+finalHost.Host, item.depth)
					failCount++
					continue
				}
				external = true
			}
		}
		okCount++
		pr.Depth = item.depth // per-page layer for DB (feature 002 US3)
		job.appendPage(pr)

		// Discover children while within depth budget. External (whitelisted)
		// pages are leaves: never expand, so whitelisted hosts stay bounded.
		if item.depth < cr.maxDepth && !external {
			base, _ := url.Parse(pr.URL)
			for _, href := range extractLinks(pr.RawHTML, base) {
				if !job.linkIncluded(href, cr.includeURLs) {
					continue // include_url filter: only matching links are followed
				}
				cKey, cErr := urlutil.URLKey(href)
				if cErr != nil || visited[cKey] {
					continue
				}
				queue = append(queue, enqueueItem{rawURL: href, depth: item.depth + 1})
			}
		}
	}
	job.finish("completed", "")
}

// inScope applies the same-host / subdomain boundary (www-equivalent).
func (job *localJob) inScope(scopeHost, linkHost string, allowSubdomains bool) bool {
	if urlutil.SameHost(scopeHost, linkHost) {
		return true
	}
	return allowSubdomains && urlutil.SubdomainAllowed(scopeHost, linkHost)
}

// linkIncluded applies the include_url substring filter to discovered links
// (case-insensitive). Empty filter list = everything passes.
func (job *localJob) linkIncluded(rawURL string, includeURLs []string) bool {
	if len(includeURLs) == 0 {
		return true
	}
	low := strings.ToLower(rawURL)
	for _, sub := range includeURLs {
		if strings.Contains(low, sub) {
			return true
		}
	}
	return false
}

func (job *localJob) addFailed(rawURL, reason string, depth int) {
	job.mu.Lock()
	job.failedPages = append(job.failedPages, &PageResult{URL: rawURL, Failed: true, Err: reason})
	job.mu.Unlock()
}

func (job *localJob) appendPage(pr *PageResult) {
	job.mu.Lock()
	job.completedPages = append(job.completedPages, pr)
	job.mu.Unlock()
}

// finish records the terminal state.
func (job *localJob) finish(status, msg string) {
	job.mu.Lock()
	job.status = status
	job.err = msg
	job.mu.Unlock()
}

// poll drains completed pages since the last call (paging semantics).
func (job *localJob) poll() *CrawlBatch {
	job.mu.Lock()
	defer job.mu.Unlock()
	batch := &CrawlBatch{
		JobID: job.id, Status: job.status,
		Completed: len(job.completedPages), Total: job.visitedCount,
		Pages: job.completedPages,
	}
	job.completedPages = nil
	return batch
}

func (job *localJob) failed() []*PageResult {
	job.mu.Lock()
	defer job.mu.Unlock()
	return job.failedPages
}

// --- localJobs public surface (Engine interface support for local engines) ---

// PollCrawl implements the poll half of the job lifecycle.
func (j *localJobs) PollCrawl(ctx context.Context, jobID string) (*CrawlBatch, error) {
	job, err := j.get(jobID)
	if err != nil {
		return nil, err
	}
	return job.poll(), nil
}

// FetchErrors returns the cumulative failed page list of a job.
func (j *localJobs) FetchErrors(ctx context.Context, jobID string) ([]*PageResult, error) {
	job, err := j.get(jobID)
	if err != nil {
		return nil, err
	}
	return job.failed(), nil
}

// CancelJob cancels a local job and removes it from the registry.
func (j *localJobs) CancelJob(ctx context.Context, jobID string) error {
	j.mu.Lock()
	job, ok := j.jobs[jobID]
	if ok {
		delete(j.jobs, jobID)
	}
	j.mu.Unlock()
	if !ok {
		return fmt.Errorf("本地抓取任务 %s 不存在", jobID)
	}
	job.cancel()
	return nil
}

// extractLinks pulls http(s) hrefs from a raw HTML snapshot, resolved against
// the page base URL (used for BFS discovery, FR-003).
func extractLinks(rawHTML string, base *url.URL) []string {
	if strings.TrimSpace(rawHTML) == "" {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			return
		}
		ref, err := url.Parse(href)
		if err != nil {
			return
		}
		resolved := base.ResolveReference(ref)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return
		}
		if urlutil.IsNonWebURL(resolved.String()) {
			return // non-web file links are recorded but never fetched (FR-022)
		}
		abs := resolved.String()
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	})
	return out
}
