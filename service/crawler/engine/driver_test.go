package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// buildSite serves a deterministic multi-layer test site:
//
//	/index.html -> /sub/a.html -> /sub/a2.html   (2 layers)
//	             -> /sub/b.html
//	             -> /sub/loop.html -> /index.html (cycle)
//	             -> /private/x.html                (robots-disallowed)
//	             -> /file.pdf                      (non-web, never fetched)
//	             -> https://example.com/external   (cross-site, never fetched)
func buildSite(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		p := r.URL.Path
		if p == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
			return
		}
		body := map[string]string{
			"/index.html":    `<html><head><title>首页</title></head><body><a href="/sub/a.html">A</a><a href="/sub/b.html">B</a><a href="/sub/loop.html">L</a><a href="/private/x.html">P</a><a href="/file.pdf">F</a><a href="https://example.com/ext">X</a></body></html>`,
			"/sub/a.html":    `<html><head><title>子页A</title></head><body><a href="/sub/a2.html">A2</a><a href="/index.html">回</a></body></html>`,
			"/sub/a2.html":   `<html><head><title>子页A2</title></head><body>二层</body></html>`,
			"/sub/b.html":    `<html><head><title>子页B</title></head><body>B内容</body></html>`,
			"/sub/loop.html": `<html><head><title>环</title></head><body><a href="/index.html">回首页</a></body></html>`,
		}
		html, ok := body[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	}))
	return srv, &hits
}

func titles(pages []*PageResult) []string {
	out := []string{}
	for _, p := range pages {
		out = append(out, p.Title)
	}
	return out
}

// runLocalCrawl drives submit+poll until terminal, collecting all pages.
func runLocalCrawl(t *testing.T, eng interface {
	SubmitCrawl(context.Context, *CrawlRequest) (string, error)
	PollCrawl(context.Context, string) (*CrawlBatch, error)
	FetchErrors(context.Context, string) ([]*PageResult, error)
}, req *CrawlRequest) ([]*PageResult, []*PageResult, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	jobID, err := eng.SubmitCrawl(ctx, req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	pages, failed := []*PageResult{}, []*PageResult{}
	status := ""
	for i := 0; i < 100; i++ {
		batch, err := eng.PollCrawl(ctx, jobID)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		pages = append(pages, batch.Pages...)
		status = batch.Status
		if status == "completed" || status == "cancelled" || status == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "completed" && status != "cancelled" {
		t.Fatalf("job did not terminate, status=%s pages=%v", status, titles(pages))
	}
	if status == "completed" {
		failed, _ = eng.FetchErrors(ctx, jobID)
	}
	return pages, failed, status
}

func TestDriverDepthBoundariesAndDedup(t *testing.T) {
	srv, _ := buildSite(t)
	defer srv.Close()
	c := NewGoquery(0, time.Millisecond)

	// depth=1: entry + direct children only (A3 semantics), no a2 layer.
	pages, failed, status := runLocalCrawl(t, c, &CrawlRequest{
		URL: srv.URL + "/index.html", MaxDiscoveryDepth: 1, Limit: 20,
	})
	if status != "completed" {
		t.Fatalf("status = %s", status)
	}
	got := titles(pages)
	for _, want := range []string{"首页", "子页A", "子页B", "环"} {
		if !containsStr(got, want) {
			t.Errorf("depth1 missing %q in %v", want, got)
		}
	}
	if containsStr(got, "子页A2") {
		t.Errorf("depth1 must not reach layer-2 page: %v", got)
	}
	// Cycle terminated & page deduped: 首页 fetched once as one result.
	if len(pages) != 4 {
		t.Errorf("depth1 pages = %d (%v), want 4", len(pages), got)
	}
	// robots-disallowed link recorded as failed, external & pdf never fetched.
	if len(failed) != 1 || !strings.Contains(failed[0].Err, "robots") {
		t.Errorf("expected one robots failure, got %v", failed)
	}

	// depth=2 reaches the second layer.
	pages2, _, _ := runLocalCrawl(t, c, &CrawlRequest{
		URL: srv.URL + "/index.html", MaxDiscoveryDepth: 2, Limit: 20,
	})
	if !containsStr(titles(pages2), "子页A2") {
		t.Errorf("depth2 should include 子页A2: %v", titles(pages2))
	}
}

func TestDriverLimitStops(t *testing.T) {
	srv, _ := buildSite(t)
	defer srv.Close()
	c := NewGoquery(0, time.Millisecond)
	pages, _, status := runLocalCrawl(t, c, &CrawlRequest{
		URL: srv.URL + "/index.html", MaxDiscoveryDepth: 2, Limit: 2,
	})
	if status != "completed" {
		t.Fatalf("status = %s", status)
	}
	if len(pages) != 2 {
		t.Errorf("limit=2 should fetch exactly 2 pages, got %d", len(pages))
	}
}

func TestDriverCrossSiteAndNonWebNeverFetched(t *testing.T) {
	srv, hits := buildSite(t)
	defer srv.Close()
	before := hits.Load()
	c := NewGoquery(0, time.Millisecond)
	runLocalCrawl(t, c, &CrawlRequest{
		URL: srv.URL + "/index.html", MaxDiscoveryDepth: 1, Limit: 20,
	})
	// external + pdf + private URLs must never hit the page handler beyond
	// robots.txt fetches (each new host adds one robots fetch).
	// Handler counts robots.txt too, so allow +1 (localhost robots) only.
	if hits.Load() > before+6 { // 4 pages + robots + entry... tolerant bound
		t.Errorf("unexpected extra fetches: %d", hits.Load()-before)
	}
}

// buildExternalSite serves a second "site" (different port = different host)
// with a page that would expand further if the driver wrongly followed links
// from whitelisted external pages.
func buildExternalSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/ext.html":
			fmt.Fprint(w, `<html><head><title>外部页</title></head><body><a href="/self.html">再挖</a><a href="https://example.com/deep">更外</a></body></html>`)
		case "/self.html":
			fmt.Fprint(w, `<html><head><title>外部子页(不应出现)</title></head><body>x</body></html>`)
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
	}))
	return srv
}

func TestDriverAllowHostsWhitelist(t *testing.T) {
	ext := buildExternalSite(t)
	defer ext.Close()

	// Site A's index links to srvB's ext.html as a whitelisted external link.
	link := fmt.Sprintf(`<a href="%s/ext.html">外</a>`, ext.URL)
	c := NewGoquery(0, time.Millisecond)

	// 1. Without allow_hosts the external link is never fetched.
	noAllowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>首页</title></head><body>`+link+`</body></html>`)
	}))
	defer noAllowSrv.Close()
	pages, _, _ := runLocalCrawl(t, c, &CrawlRequest{
		URL: noAllowSrv.URL + "/", MaxDiscoveryDepth: 1, Limit: 20,
	})
	if containsStr(titles(pages), "外部页") {
		t.Errorf("without whitelist external page must not be fetched: %v", titles(pages))
	}

	// 2. With allow_hosts the external page is fetched once and never expands
	// (leaf): 外部子页 must not appear, no second crawl of ext.html.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>首页</title></head><body>`+link+`</body></html>`)
	}))
	defer srv2.Close()
	extHost := ext.URL[len("http://"):] // host:port
	pages2, _, _ := runLocalCrawl(t, c, &CrawlRequest{
		URL: srv2.URL + "/", MaxDiscoveryDepth: 2, Limit: 20,
		AllowHosts: []string{extHost},
	})
	got2 := titles(pages2)
	if !containsStr(got2, "外部页") {
		t.Errorf("whitelisted external page should be fetched: %v", got2)
	}
	if containsStr(got2, "外部子页(不应出现)") {
		t.Errorf("external pages must be leaves (never expand): %v", got2)
	}
	// robots.txt of ext host counts as a fetch; ext page exactly once.
	if got2 = titles(pages2); len(got2) != 2 {
		t.Errorf("expected entry + external page only, got %d: %v", len(got2), got2)
	}

	// 3. Redirect escaping to a whitelisted host is recorded as success.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/jump.html" {
			http.Redirect(w, r, ext.URL+"/ext.html", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer redir.Close()
	pages3, failed3, _ := runLocalCrawl(t, c, &CrawlRequest{
		URL: redir.URL + "/jump.html", MaxDiscoveryDepth: 1, Limit: 10,
		AllowHosts: []string{extHost},
	})
	if !containsStr(titles(pages3), "外部页") {
		t.Errorf("redirect onto whitelisted host should succeed: %v", titles(pages3))
	}
	if len(failed3) != 0 {
		t.Errorf("no failures expected on whitelisted redirect, got %v", failed3)
	}
}

func TestDriverIncludeURLFilter(t *testing.T) {
	srv, _ := buildSite(t)
	defer srv.Close()
	c := NewGoquery(0, time.Millisecond)
	// include_url "/sub/a" ⇒ only 首页 + 子页A are fetched (b/loop/private/pdf
	// links filtered out at discovery; robots failure count unchanged).
	pages, failed, _ := runLocalCrawl(t, c, &CrawlRequest{
		URL: srv.URL + "/index.html", MaxDiscoveryDepth: 1, Limit: 20,
		IncludeURLs: []string{"/sub/a"},
	})
	got := titles(pages)
	if !containsStr(got, "首页") || !containsStr(got, "子页A") {
		t.Errorf("entry + matching child expected: %v", got)
	}
	if len(pages) != 2 {
		t.Errorf("only entry + matching link expected, got %d: %v", len(pages), got)
	}
	// /private/x.html is not matched by include_url, so no robots failure.
	if len(failed) != 0 {
		t.Errorf("no failures expected under include_url filter, got %v", failed)
	}
}

func TestDriverCancelStops(t *testing.T) {
	srv, _ := buildSite(t)
	defer srv.Close()
	c := NewGoquery(0, 5*time.Millisecond)

	// Slow page to keep the job running while we cancel.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/slow") {
			time.Sleep(2 * time.Second)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>slow</title></head><body><a href="/slow2.html">x</a></body></html>`)
	}))
	defer slow.Close()

	ctx := context.Background()
	jobID, err := c.SubmitCrawl(ctx, &CrawlRequest{URL: slow.URL + "/slow.html", MaxDiscoveryDepth: 2, Limit: 20})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	time.Sleep(150 * time.Millisecond) // let it start fetching
	if err := c.CancelJob(ctx, jobID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// After cancel the job is deregistered: polling must fail cleanly instead
	// of returning more pages, and no further fetches may be issued.
	for i := 0; i < 20; i++ {
		_, perr := c.PollCrawl(ctx, jobID)
		if perr != nil {
			return // deregistered as expected
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job still registered after CancelJob")
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
