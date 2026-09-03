package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// scrapeFormats are the output formats requested from Firecrawl: markdown for
// the body contract + rawHtml for the backup snapshot (research.md §6).
const scrapeFormatsJSON = `["markdown","rawHtml"]`

// nonWebPathRegex excludes file links from crawl discovery (FR-022), matching
// the full URL so query strings do not bypass the filter.
const nonWebPathRegex = `.*\.(pdf|jpe?g|png|gif|webp|svg|mp4|avi|mov|zip|rar|7z|gz|docx?|xlsx?|pptx?)(\?.*)?$`

// Client implements Engine against the Firecrawl v2 HTTP API
// (https://api.firecrawl.dev; base URL configurable for self-hosting).
type Client struct {
	baseURL   string
	apiKey    string
	http      *http.Client
	retries   int           // API-call retry count (FR-023, research §4)
	retryWait time.Duration // sleep between API-call retries
}

// New returns a Firecrawl client. retries/retryWait apply to API-call failures
// (network / 5xx / 429); per-page retries are handled inside Firecrawl.
func New(baseURL, apiKey string, retries int, retryWait time.Duration) *Client {
	if baseURL == "" {
		baseURL = "https://api.firecrawl.dev"
	}
	return &Client{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		apiKey:    apiKey,
		http:      &http.Client{Timeout: 90 * time.Second},
		retries:   retries,
		retryWait: retryWait,
	}
}

// request performs a JSON API call with retries on transport/5xx/429 failures.
func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	var lastErr error
	attempts := c.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryWait):
			}
		}
		lastErr = c.doOnce(ctx, method, path, body, out)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("firecrawl api %s %s after %d attempt(s): %w", method, path, attempts, lastErr)
}

func (c *Client) doOnce(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response (http %d): %w", resp.StatusCode, err)
	}
	return nil
}

// Scrape implements Engine: single-page fetch (task depth 0).
func (c *Client) Scrape(ctx context.Context, rawURL string) (*PageResult, error) {
	// formats: markdown (body) + html (cleaned snapshot). Firecrawl rawHtml was
	// measured >120s on WeChat pages vs ~8s for cleaned html (research §6 note);
	// rawHtml backup can be revisited later without contract change.
	payload := map[string]any{
		"url":             rawURL,
		"formats":         []string{"markdown", "html"},
		"onlyMainContent": false,
	}
	var out struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string `json:"markdown"`
			RawHTML  string `json:"html"`
			Metadata struct {
				Title      string `json:"title"`
				SourceURL  string `json:"sourceURL"`
				StatusCode int    `json:"statusCode"`
				Error      string `json:"error"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := c.request(ctx, http.MethodPost, "/v2/scrape", payload, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("firecrawl scrape failed (success=false)")
	}
	pr := &PageResult{
		URL:        firstNonEmpty(out.Data.Metadata.SourceURL, rawURL),
		Title:      out.Data.Metadata.Title,
		Markdown:   out.Data.Markdown,
		RawHTML:    out.Data.RawHTML,
		StatusCode: out.Data.Metadata.StatusCode,
		Err:        out.Data.Metadata.Error,
	}
	// A statusCode of 0 means no page-level info was returned.
	if pr.StatusCode == 0 {
		pr.StatusCode = 200
	}
	return pr, nil
}

// SubmitCrawl implements Engine: async whole-site crawl (task depth >= 1).
func (c *Client) SubmitCrawl(ctx context.Context, req *CrawlRequest) (string, error) {
	payload := map[string]any{
		"url":                   req.URL,
		"maxDiscoveryDepth":     req.MaxDiscoveryDepth,
		"crawlEntireDomain":     true,
		"allowSubdomains":       req.AllowSubdomains,
		"allowExternalLinks":    false,
		"ignoreQueryParameters": false,
		"sitemap":               "skip",
		"ignoreRobotsTxt":       false,
		"limit":                 req.Limit,
		"excludePaths":          []string{nonWebPathRegex},
		"regexOnFullURL":        true,
		"scrapeOptions": map[string]any{
			"formats":         []string{"markdown", "html"},
			"onlyMainContent": false,
		},
	}
	if req.DelaySeconds > 0 {
		payload["delay"] = req.DelaySeconds
	}
	var out struct {
		Success bool   `json:"success"`
		ID      string `json:"id"`
	}
	if err := c.request(ctx, http.MethodPost, "/v2/crawl", payload, &out); err != nil {
		return "", err
	}
	if !out.Success || out.ID == "" {
		return "", fmt.Errorf("firecrawl crawl submit failed (success=%v id=%q)", out.Success, out.ID)
	}
	return out.ID, nil
}

// PollCrawl implements Engine: fetch job progress + completed pages (paged).
func (c *Client) PollCrawl(ctx context.Context, jobID string) (*CrawlBatch, error) {
	path := "/v2/crawl/" + jobID
	batch := &CrawlBatch{JobID: jobID, Pages: []*PageResult{}}
	for {
		var out struct {
			Status    string `json:"status"`
			Total     int    `json:"total"`
			Completed int    `json:"completed"`
			Next      string `json:"next"`
			Data      []struct {
				Markdown string `json:"markdown"`
				RawHTML  string `json:"html"`
				Metadata struct {
					Title      string `json:"title"`
					SourceURL  string `json:"sourceURL"`
					StatusCode int    `json:"statusCode"`
				} `json:"metadata"`
			} `json:"data"`
		}
		if err := c.request(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}
		batch.Status = out.Status
		batch.Total = out.Total
		batch.Completed = out.Completed
		for _, d := range out.Data {
			pr := &PageResult{
				URL:        d.Metadata.SourceURL,
				Title:      d.Metadata.Title,
				Markdown:   d.Markdown,
				RawHTML:    d.RawHTML,
				StatusCode: d.Metadata.StatusCode,
			}
			if pr.StatusCode == 0 {
				pr.StatusCode = 200
			}
			batch.Pages = append(batch.Pages, pr)
		}
		if out.Next == "" {
			return batch, nil
		}
		// Follow paging: next is an absolute URL carrying ?skip=N.
		path = strings.TrimPrefix(out.Next, c.baseURL)
		if path == out.Next {
			return nil, fmt.Errorf("firecrawl next paging URL outside base: %s", out.Next)
		}
	}
}

// FetchErrors implements Engine: pages Firecrawl could not scrape
// (network errors, timeouts, robots blocks — crawl errors endpoint).
func (c *Client) FetchErrors(ctx context.Context, jobID string) ([]*PageResult, error) {
	var out struct {
		Success bool `json:"success"`
		Data    []struct {
			URL     string `json:"url"`
			Source  string `json:"sourceURL"`
			Error   string `json:"error"`
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"data"`
	}
	if err := c.request(ctx, http.MethodGet, "/v2/crawl/"+jobID+"/errors", nil, &out); err != nil {
		return nil, err
	}
	results := make([]*PageResult, 0, len(out.Data))
	for _, d := range out.Data {
		results = append(results, &PageResult{
			URL:    firstNonEmpty(d.URL, d.Source),
			Failed: true,
			Err:    firstNonEmpty(d.Error, d.Message, d.Code),
		})
	}
	return results, nil
}

// CancelJob implements Engine: cancel a running crawl job (FR-026 stop).
func (c *Client) CancelJob(ctx context.Context, jobID string) error {
	var out struct {
		Status string `json:"status"`
	}
	return c.request(ctx, http.MethodDelete, "/v2/crawl/"+jobID, nil, &out)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
