package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sg.scout/config"
)

// crawl4aiSyncPath is the synchronous single-page endpoint of the local
// Crawl4AI server (research.md §5: HTTP API blocks deep-crawl params, so depth
// is driven by our BFS driver over per-page /crawl calls).
const crawl4aiSyncPath = "/crawl"

// Crawl4AIClient fetches single pages from a local Crawl4AI Docker server
// (feature 002 FR-008, research.md §5). It only uses the synchronous POST
// /crawl endpoint; page_timeout stays well under the server wall-clock.
type Crawl4AIClient struct {
	baseURL   string
	token     string
	http      *http.Client
	retries   int
	retryWait time.Duration
	jobs      *localJobs // BFS job registry (US3 driver)
}

// NewCrawl4AI creates the local Crawl4AI engine adapter.
func NewCrawl4AI(baseURL, token string, retries int, retryWait time.Duration) *Crawl4AIClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11235"
	}
	if retries < 0 {
		retries = 0
	}
	if retryWait <= 0 {
		retryWait = 2 * time.Second
	}
	return &Crawl4AIClient{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		token:     token,
		http:      &http.Client{Timeout: 90 * time.Second},
		retries:   retries,
		retryWait: retryWait,
		jobs:      newLocalJobs(),
	}
}

// crawlResponse mirrors the synchronous /crawl response (v0.9.3 schemas).
type crawlResponse struct {
	Success bool           `json:"success"`
	Results []crawl4aiPage `json:"results"`
	Error   string         `json:"error"`
}

type crawl4aiPage struct {
	URL                  string `json:"url"`
	Success              bool   `json:"success"`
	StatusCode           int    `json:"status_code"`
	RedirectedStatusCode int    `json:"redirected_status_code"`
	RedirectedURL        string `json:"redirected_url"`
	ErrorMessage         string `json:"error_message"`
	Metadata             struct {
		Title string `json:"title"`
	} `json:"metadata"`
	HTML     string `json:"html"`
	Markdown *struct {
		RawMarkdown string `json:"raw_markdown"`
	} `json:"markdown"`
}

// Scrape implements Engine: one synchronous POST /crawl for a single URL.
func (c *Crawl4AIClient) Scrape(ctx context.Context, rawURL string) (*PageResult, error) {
	payload := map[string]any{
		"urls": []string{rawURL},
		"crawler_config": map[string]any{
			"cache_mode":   "bypass",
			"page_timeout": 60000,
		},
	}
	var out crawlResponse
	if err := c.request(ctx, http.MethodPost, crawl4aiSyncPath, payload, &out); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return &PageResult{URL: rawURL, Failed: true, Err: err.Error()}, nil
	}
	if !out.Success {
		msg := out.Error
		if msg == "" {
			msg = "crawl4ai 返回 success=false"
		}
		return &PageResult{URL: rawURL, Failed: true, Err: msg}, nil
	}
	if len(out.Results) == 0 {
		return &PageResult{URL: rawURL, Failed: true, Err: "crawl4ai 未返回结果"}, nil
	}
	p := out.Results[0]
	pr := &PageResult{
		URL:     rawURL,
		RawHTML: p.HTML,
	}
	if !p.Success {
		pr.Failed = true
		pr.Err = firstText(p.ErrorMessage, "crawl4ai 单页抓取失败")
		return pr, nil
	}
	pr.Title = p.Metadata.Title
	if p.Markdown != nil {
		pr.Markdown = p.Markdown.RawMarkdown
	}
	// Final URL: engine follows redirects server-side.
	if p.RedirectedURL != "" {
		pr.URL = p.RedirectedURL
	} else if p.URL != "" {
		pr.URL = p.URL
	}
	// Status: prefer final target status, fall back to the first-response code,
	// then to 200 when absent (mirrors the firecrawl adapter convention).
	switch {
	case p.RedirectedStatusCode > 0:
		pr.StatusCode = p.RedirectedStatusCode
	case p.StatusCode > 0:
		pr.StatusCode = p.StatusCode
	default:
		pr.StatusCode = 200
	}
	if pr.StatusCode >= 400 {
		pr.Err = fmt.Sprintf("http %d", pr.StatusCode)
	}
	return pr, nil
}

// Ping probes the server health endpoint (no auth needed, v0.9.3 /health).
func (c *Crawl4AIClient) Ping(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Crawl4AIProbePing probes the configured server with a short timeout — used
// by the engines registry to report available=true only when it is reachable.
func Crawl4AIProbePing() bool {
	ec := config.Cfg.Crawler.Engine.Crawl4AI
	if ec.BaseURL == "" {
		return false
	}
	probe := &Crawl4AIClient{
		baseURL: strings.TrimSuffix(ec.BaseURL, "/"),
		token:   ec.APIToken,
		http:    &http.Client{Timeout: 1200 * time.Millisecond},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()
	return probe.Ping(ctx)
}

// request performs a JSON API call with retries on transport/5xx/429.
func (c *Crawl4AIClient) request(ctx context.Context, method, path string, body any, out any) error {
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
	return lastErr
}

func (c *Crawl4AIClient) doOnce(ctx context.Context, method, path string, body any, out any) error {
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("crawl4ai 服务不可达（%s）: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("crawl4ai http %d: %s", resp.StatusCode, truncateText(string(data), 200))
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("crawl4ai 认证失败（http %d）：检查 api_token", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("crawl4ai http %d: %s", resp.StatusCode, truncateText(string(data), 200))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("crawl4ai decode (http %d): %w", resp.StatusCode, err)
	}
	return nil
}

func firstText(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Depth job methods: driven by the shared BFS driver (US3, research §2).
func (c *Crawl4AIClient) SubmitCrawl(ctx context.Context, req *CrawlRequest) (string, error) {
	return c.jobs.submit(ctx, req, c.Scrape, nil)
}

func (c *Crawl4AIClient) PollCrawl(ctx context.Context, jobID string) (*CrawlBatch, error) {
	return c.jobs.PollCrawl(ctx, jobID)
}

func (c *Crawl4AIClient) FetchErrors(ctx context.Context, jobID string) ([]*PageResult, error) {
	return c.jobs.FetchErrors(ctx, jobID)
}

func (c *Crawl4AIClient) CancelJob(ctx context.Context, jobID string) error {
	return c.jobs.CancelJob(ctx, jobID)
}
