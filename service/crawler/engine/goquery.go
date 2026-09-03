package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Page-level failure categories (run_page.error vocabulary, research §2.1).
var (
	errRobotsBlocked = errors.New("robots.txt 禁止抓取")
	errParseFailed   = errors.New("页面解析失败")
)

// maxPageBytes caps the raw response body kept (backup snapshot guard).
const maxPageBytes = 20 << 20

// GoqueryClient is the local direct-fetch engine (feature 002 FR-007,
// research.md §2.1): stdlib HTTP + goquery parsing, no external service, no
// JS rendering. Depth crawl (US3) is driven by the shared BFS driver over the
// same fetcher; the async job methods return an explicit not-wired error until
// the driver lands (US3 replaces them).
type GoqueryClient struct {
	http      *http.Client
	retries   int           // transient fetch retries (FR-023 API-layer mapping)
	retryWait time.Duration // sleep between retries
	robots    *robotsCache
	jobs      *localJobs // BFS job registry (US3 driver)
}

// NewGoquery creates the local direct-fetch engine.
func NewGoquery(retries int, retryWait time.Duration) *GoqueryClient {
	if retries < 0 {
		retries = 0
	}
	if retryWait <= 0 {
		retryWait = 2 * time.Second
	}
	return &GoqueryClient{
		http:      &http.Client{Timeout: 60 * time.Second},
		retries:   retries,
		retryWait: retryWait,
		robots:    newRobotsCache(),
		jobs:      newLocalJobs(),
	}
}

// Scrape implements Engine: single-page direct fetch (task depth 0 + failed
// page retry source). Robots.txt is respected (FR-007): disallowed URLs are
// reported as page-level failures, never fetched.
func (c *GoqueryClient) Scrape(ctx context.Context, rawURL string) (*PageResult, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("goquery: 无效 URL %q", rawURL)
	}
	if !c.robots.allowed(c.http, u) {
		return &PageResult{URL: rawURL, Failed: true, Err: errRobotsBlocked.Error()}, nil
	}
	resp, raw, err := c.fetch(ctx, u.String())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return &PageResult{URL: rawURL, Failed: true, Err: err.Error()}, nil
	}
	pr := &PageResult{
		URL:        resp.Request.URL.String(), // final URL after redirects
		RawHTML:    string(raw),
		StatusCode: resp.StatusCode,
	}
	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
	if derr != nil {
		pr.Failed = true
		pr.Err = errParseFailed.Error()
		return pr, nil
	}
	pr.Title = strings.TrimSpace(doc.Find("title").First().Text())
	pr.Markdown = htmlToMarkdown(doc)
	if resp.StatusCode >= 400 {
		pr.Err = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return pr, nil
}

// fetch performs the HTTP GET with retries on transport errors and 5xx
// (FR-023 retry semantics at the engine call layer).
func (c *GoqueryClient) fetch(ctx context.Context, rawURL string) (*http.Response, []byte, error) {
	var lastErr error
	attempts := c.retries + 1
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(c.retryWait):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue // transport error -> retry
		}
		// Decode legacy charsets (GBK etc.) to UTF-8 so parsed content and DB
		// writes stay valid utf8mb4 (feature 002 ahjxyy GBK-site finding).
		body, rerr := decodeBody(resp)
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue // 5xx -> retry
		}
		return resp, body, nil
	}
	return nil, nil, fmt.Errorf("goquery fetch after %d attempt(s): %w", attempts, lastErr)
}

// decodeBody reads the response body (capped) and decodes legacy charsets
// (GBK/GB2312/Big5 …) into UTF-8. DetermineEncoding honours the Content-Type
// charset and falls back to sniffing the first bytes / <meta>.
func decodeBody(resp *http.Response) ([]byte, error) {
	ct := resp.Header.Get("Content-Type")
	br := bufio.NewReader(io.LimitReader(resp.Body, maxPageBytes))
	head, _ := br.Peek(1024)
	enc, _, _ := charset.DetermineEncoding(head, ct)
	if isUTF8(enc) {
		// Fast path: plain UTF-8 (or ASCII) — read through unchanged.
		return io.ReadAll(br)
	}
	dec := transform.NewReader(br, enc.NewDecoder())
	return io.ReadAll(dec)
}

// isUTF8 reports whether the encoding is UTF-8/ASCII (no conversion needed).
func isUTF8(enc encoding.Encoding) bool {
	if enc == nil {
		return true
	}
	return enc == encoding.Nop || enc == xunicode.UTF8
}

// Depth job methods: driven by the shared BFS driver (US3, research §2).
func (c *GoqueryClient) SubmitCrawl(ctx context.Context, req *CrawlRequest) (string, error) {
	return c.jobs.submit(ctx, req, c.Scrape, c.robots)
}

func (c *GoqueryClient) PollCrawl(ctx context.Context, jobID string) (*CrawlBatch, error) {
	return c.jobs.PollCrawl(ctx, jobID)
}

func (c *GoqueryClient) FetchErrors(ctx context.Context, jobID string) ([]*PageResult, error) {
	return c.jobs.FetchErrors(ctx, jobID)
}

func (c *GoqueryClient) CancelJob(ctx context.Context, jobID string) error {
	return c.jobs.CancelJob(ctx, jobID)
}
