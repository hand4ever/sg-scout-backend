// Package engine abstracts the crawl backend so the crawler module can swap
// providers (Firecrawl v2 by default; research.md rev2 §0). Results are
// engine-neutral: the adapter maps provider output into these structures.
package engine

import "context"

// PageResult is one fetched page in engine-neutral form.
type PageResult struct {
	// URL is the final page URL after canonical redirects (metadata.sourceURL).
	URL string
	// Title is the page title from engine metadata.
	Title string
	// Markdown is the body content as markdown (onlyMainContent=false so it
	// includes nav/footer text — spec A5 fingerprint scope).
	Markdown string
	// RawHTML is the original unmodified HTML snapshot (backup only, spec A8).
	RawHTML string
	// StatusCode is the TARGET page HTTP status (2xx/304 = clean; 404 = offline;
	// other 4xx/5xx = failed). Distinct from the engine API request status.
	StatusCode int
	// Depth is the layer at which the page was discovered (0 = entry/Scrape;
	// the local BFS driver sets real layers; cloud engines leave 0).
	Depth int
	// Failed marks pages the engine itself could not scrape (network/timeout/
	// robots) — surfaced via crawl errors, not the data array.
	Failed bool
	// Err carries a human-readable reason when Failed or StatusCode is an error.
	Err string
}

// CrawlRequest carries crawl job parameters mapped from task configuration
// (FR-003/011/016/018/022 — research.md §1-§3).
type CrawlRequest struct {
	URL               string
	MaxDiscoveryDepth int
	AllowSubdomains   bool
	AllowHosts        []string // external hosts allowed past the same-site boundary; fetched pages never expand (leaf)
	IgnoreRobots      bool     // true = fetch robots-disallowed paths (e.g. wechat articles)
	IncludeURLs       []string // URL substrings; only discovered links containing one are followed (entry always fetched)
	Limit             int
	// DelaySeconds spaces requests (FR-018 throttle; forces concurrency 1).
	DelaySeconds int
}

// CrawlBatch is a page of results fetched during polling.
type CrawlBatch struct {
	JobID     string
	Status    string // scraping | completed | cancelled | failed
	Completed int
	Total     int
	Pages     []*PageResult
}

// Engine is the crawl backend contract (FR-030 engine abstraction).
type Engine interface {
	// Scrape fetches a single URL synchronously (task depth 0).
	Scrape(ctx context.Context, url string) (*PageResult, error)
	// SubmitCrawl starts an asynchronous site crawl and returns its job id.
	SubmitCrawl(ctx context.Context, req *CrawlRequest) (string, error)
	// PollCrawl returns the current job status and any pages available so far.
	PollCrawl(ctx context.Context, jobID string) (*CrawlBatch, error)
	// FetchErrors returns URLs the engine failed to scrape in a crawl job.
	FetchErrors(ctx context.Context, jobID string) ([]*PageResult, error)
	// CancelJob cancels a running crawl job (FR-026 stop).
	CancelJob(ctx context.Context, jobID string) error
}
