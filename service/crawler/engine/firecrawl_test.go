package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	cli := New(srv.URL, "fc-test-key", 2, 5*time.Millisecond)
	t.Cleanup(srv.Close)
	return srv, cli
}

func TestScrape_PageStatusLayered(t *testing.T) {
	// Two status layers: API success=true even when target page returned 404.
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/scrape" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fc-test-key" {
			t.Error("missing bearer auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# 404","metadata":{"title":"Not Found","sourceURL":"https://example.com/gone","statusCode":404}}}`))
	})
	pr, err := cli.Scrape(context.Background(), "https://example.com/gone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.StatusCode != 404 {
		t.Errorf("expected page statusCode 404 (target layer), got %d", pr.StatusCode)
	}
	if pr.URL != "https://example.com/gone" || pr.Title != "Not Found" {
		t.Errorf("unexpected page meta: %+v", pr)
	}
}

func TestScrape_ApiRetryOn5xx(t *testing.T) {
	var calls int32
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"ok","metadata":{"sourceURL":"https://example.com","statusCode":200}}}`))
	})
	pr, err := cli.Scrape(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if pr.Markdown != "ok" {
		t.Errorf("expected markdown ok, got %q", pr.Markdown)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 attempts (1+2 retries), got %d", got)
	}
}

func TestScrape_RetryExhausted(t *testing.T) {
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	})
	if _, err := cli.Scrape(context.Background(), "https://example.com"); err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestSubmitCrawl_Parameters(t *testing.T) {
	var got map[string]any
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/crawl" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"id":"job-123"}`))
	})
	id, err := cli.SubmitCrawl(context.Background(), &CrawlRequest{
		URL:               "https://example.com",
		MaxDiscoveryDepth: 2,
		AllowSubdomains:   true,
		Limit:             10,
		DelaySeconds:      1,
	})
	if err != nil || id != "job-123" {
		t.Fatalf("submit: id=%q err=%v", id, err)
	}
	// JSON numbers decode as float64; compare in that domain.
	checks := map[string]any{
		"maxDiscoveryDepth":    float64(2),
		"allowSubdomains":      true,
		"crawlEntireDomain":    true,
		"allowExternalLinks":   false,
		"ignoreQueryParameters": false,
		"sitemap":              "skip",
		"ignoreRobotsTxt":      false,
		"limit":                float64(10),
		"delay":                float64(1),
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("crawl param %s: expected %v, got %v", k, want, got[k])
		}
	}
	so, ok := got["scrapeOptions"].(map[string]any)
	if !ok {
		t.Fatal("missing scrapeOptions")
	}
	if so["onlyMainContent"] != false {
		t.Errorf("expected onlyMainContent=false, got %v", so["onlyMainContent"])
	}
	ex, ok := got["excludePaths"].([]any)
	if !ok || len(ex) == 0 {
		t.Error("expected excludePaths for non-web files (FR-022)")
	}
}

func TestPollCrawl_Paging(t *testing.T) {
	var page1, page2 bool
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !page1 {
			page1 = true
			_, _ = w.Write([]byte(`{"status":"scraping","total":2,"completed":1,"next":"` + srvURL(t, r) + `/v2/crawl/job-1?skip=1","data":[{"markdown":"a","metadata":{"sourceURL":"https://example.com/a","statusCode":200}}]}`))
			return
		}
		if !page2 {
			page2 = true
			_, _ = w.Write([]byte(`{"status":"completed","total":2,"completed":2,"data":[{"markdown":"b","metadata":{"sourceURL":"https://example.com/b","statusCode":200}}]}`))
			return
		}
		t.Fatal("unexpected third poll call")
	})
	batch, err := cli.PollCrawl(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(batch.Pages) != 2 {
		t.Errorf("expected 2 paged pages, got %d", len(batch.Pages))
	}
	if batch.Status != "completed" || batch.Total != 2 {
		t.Errorf("unexpected batch meta: %+v", batch)
	}
}

// srvURL reconstructs the mock base so `next` paging points back at the test server.
func srvURL(t *testing.T, r *http.Request) string {
	t.Helper()
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func TestFetchErrors(t *testing.T) {
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/crawl/job-9/errors" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"url":"https://example.com/slow","error":"timeout","code":"TIMEOUT"}]}`))
	})
	failed, err := cli.FetchErrors(context.Background(), "job-9")
	if err != nil {
		t.Fatalf("fetch errors: %v", err)
	}
	if len(failed) != 1 || !failed[0].Failed || failed[0].URL != "https://example.com/slow" {
		t.Errorf("unexpected errors result: %+v", failed)
	}
}

func TestCancelJob(t *testing.T) {
	var deleted bool
	_, cli := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v2/crawl/job-7" {
			t.Fatalf("expected DELETE /v2/crawl/job-7, got %s %s", r.Method, r.URL.Path)
		}
		deleted = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"cancelled"}`))
	})
	if err := cli.CancelJob(context.Background(), "job-7"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !deleted {
		t.Error("cancel endpoint was not hit")
	}
}
