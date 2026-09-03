package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockCrawl4AIServer fakes the sync /crawl + /health endpoints (v0.9.3 shape).
func mockCrawl4AIServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Crawl4AIClient) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","version":"0.9.3"}`))
			return
		}
		if handler != nil {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	client := NewCrawl4AI(srv.URL, "test-token", 1, 5*time.Millisecond)
	return srv, client
}

func c4aResult(t *testing.T, w http.ResponseWriter, page map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"success": true, "results": []any{page}})
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func TestCrawl4AIScrape_OKMapping(t *testing.T) {
	srv, client := mockCrawl4AIServer(t, func(w http.ResponseWriter, r *http.Request) {
		// token must be forwarded
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		urls, _ := body["urls"].([]any)
		if len(urls) != 1 || urls[0] != "http://site.example/page" {
			http.Error(w, "urls mismatch", http.StatusBadRequest)
			return
		}
		c4aResult(t, w, map[string]any{
			"url":                    "http://site.example/final",
			"redirected_url":         "http://site.example/final",
			"success":                true,
			"status_code":            301,
			"redirected_status_code": 200,
			"metadata":               map[string]any{"title": "最终标题"},
			"html":                   "<html><body>raw</body></html>",
			"markdown":               map[string]any{"raw_markdown": "# 正文内容"},
		})
	})
	defer srv.Close()

	pr, err := client.Scrape(context.Background(), "http://site.example/page")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if pr.Failed {
		t.Fatalf("unexpected failed: %+v", pr)
	}
	if pr.URL != "http://site.example/final" {
		t.Errorf("url = %q", pr.URL)
	}
	if pr.Title != "最终标题" || !strings.Contains(pr.Markdown, "正文内容") {
		t.Errorf("title/markdown mapping wrong: %q / %q", pr.Title, pr.Markdown)
	}
	if pr.RawHTML != "<html><body>raw</body></html>" {
		t.Errorf("raw html wrong: %q", pr.RawHTML)
	}
	if pr.StatusCode != 200 {
		t.Errorf("status = %d, want 200 (redirected)", pr.StatusCode)
	}
}

func TestCrawl4AIScrape_PageFailure(t *testing.T) {
	srv, client := mockCrawl4AIServer(t, func(w http.ResponseWriter, r *http.Request) {
		c4aResult(t, w, map[string]any{
			"url": "http://site.example/blocked", "success": false,
			"error_message": "反爬拦截", "status_code": 200,
		})
	})
	defer srv.Close()

	pr, err := client.Scrape(context.Background(), "http://site.example/blocked")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if !pr.Failed || !strings.Contains(pr.Err, "反爬") {
		t.Fatalf("expected page failure with reason, got %+v", pr)
	}
}

func TestCrawl4AIScrape_Unauthorized(t *testing.T) {
	srv, client := mockCrawl4AIServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	defer srv.Close()

	pr, err := client.Scrape(context.Background(), "http://site.example/x")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if !pr.Failed || !strings.Contains(pr.Err, "认证失败") {
		t.Fatalf("expected auth failure, got %+v", pr)
	}
}

func TestCrawl4AIScrape_Retries5xxThenOK(t *testing.T) {
	var calls int32
	srv, client := mockCrawl4AIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 1 {
			http.Error(w, "busy", http.StatusInternalServerError)
			return
		}
		c4aResult(t, w, map[string]any{
			"url": "http://site.example/ok", "success": true, "status_code": 200,
			"metadata": map[string]any{"title": "OK"}, "markdown": map[string]any{"raw_markdown": "ok"},
		})
	})
	defer srv.Close()

	pr, err := client.Scrape(context.Background(), "http://site.example/ok")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if pr.Failed {
		t.Fatalf("expected success after retry: %+v", pr)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("requests = %d, want 2", calls)
	}
}

func TestCrawl4AIScrape_ServiceUnreachable(t *testing.T) {
	client := NewCrawl4AI("http://127.0.0.1:1", "t", 0, time.Millisecond) // closed port
	pr, err := client.Scrape(context.Background(), "http://site.example/x")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if !pr.Failed || !strings.Contains(pr.Err, "服务不可达") {
		t.Fatalf("expected unreachable error, got %+v", pr)
	}
}

func TestCrawl4AIPing(t *testing.T) {
	srv, client := mockCrawl4AIServer(t, nil)
	defer srv.Close()
	if !client.Ping(context.Background()) {
		t.Fatal("Ping should be true against healthy server")
	}
	dead := NewCrawl4AI("http://127.0.0.1:1", "t", 0, time.Millisecond)
	if dead.Ping(context.Background()) {
		t.Fatal("Ping should be false against dead server")
	}
}
