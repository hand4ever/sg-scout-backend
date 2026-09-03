package engine

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const samplePage = `<!DOCTYPE html>
<html><head><title>示例标题</title></head>
<body>
<nav><a href="/news">导航链接</a></nav>
<main>
<h1>主标题</h1>
<p>第一段文字内容。</p>
<ul><li>列表项A</li><li>列表项B</li></ul>
<a href="https://example.com/detail?id=1">详情页</a>
</main>
</body></html>`

func newTestClient(retries int) *GoqueryClient {
	return NewGoquery(retries, 5*time.Millisecond)
}

// TestGoqueryScrape_GBKDecoded: a legacy GBK page must come back as valid
// UTF-8 (title + markdown), not raw bytes that would corrupt utf8mb4 writes
// (feature 002 ahjxyy finding).
func TestGoqueryScrape_GBKDecoded(t *testing.T) {
	enc := simplifiedchinese.GBK.NewEncoder()
	var body string
	// Encode a GBK page through the transformer into raw bytes.
	var buf bytes.Buffer
	w := transform.NewWriter(&buf, enc)
	html := `<!DOCTYPE html><html><head><title>泾县医院-动态</title></head><body><main><h1>医院新闻标题</h1><p>正文段落内容甲。</p></main></body></html>`
	if _, err := w.Write([]byte(html)); err != nil {
		t.Fatalf("encode gbk: %v", err)
	}
	w.Close()
	body = buf.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=gbk")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	pr, err := newTestClient(0).Scrape(context.Background(), srv.URL+"/list.asp")
	if err != nil || pr.Failed {
		t.Fatalf("scrape: err=%v failed=%v msg=%s", err, pr.Failed, pr.Err)
	}
	if pr.Title != "泾县医院-动态" {
		t.Errorf("title = %q, want 泾县医院-动态 (GBK must be decoded)", pr.Title)
	}
	if !strings.Contains(pr.Markdown, "医院新闻标题") || !strings.Contains(pr.Markdown, "正文段落内容甲") {
		t.Errorf("markdown missing decoded content: %s", pr.Markdown)
	}
	if !utf8.ValidString(pr.RawHTML) {
		t.Error("RawHTML must be valid UTF-8 after decoding")
	}
}

func TestGoqueryScrape_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, samplePage)
	}))
	defer srv.Close()

	pr, err := newTestClient(1).Scrape(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if pr.Failed {
		t.Fatalf("unexpected failed: %+v", pr)
	}
	if pr.StatusCode != 200 {
		t.Errorf("status = %d, want 200", pr.StatusCode)
	}
	if pr.Title != "示例标题" {
		t.Errorf("title = %q", pr.Title)
	}
	if !strings.Contains(pr.Markdown, "主标题") || !strings.Contains(pr.Markdown, "第一段文字内容") ||
		!strings.Contains(pr.Markdown, "导航链接") || !strings.Contains(pr.Markdown, "列表项A") {
		t.Errorf("markdown missing content: %q", pr.Markdown)
	}
	if !strings.Contains(pr.Markdown, "[详情页](https://example.com/detail?id=1)") {
		t.Errorf("link not converted: %q", pr.Markdown)
	}
	if pr.RawHTML != samplePage {
		t.Errorf("raw html mismatch (len %d vs %d)", len(pr.RawHTML), len(samplePage))
	}
}

func TestGoqueryScrape_RedirectFinalURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head><title>落地页</title></head><body><p>final body</p></body></html>")
	}))
	defer srv.Close()

	pr, err := newTestClient(0).Scrape(context.Background(), srv.URL+"/start")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if !strings.HasSuffix(pr.URL, "/final") {
		t.Errorf("final url = %q, want /final", pr.URL)
	}
	if !strings.Contains(pr.Markdown, "final body") {
		t.Errorf("markdown from final page expected: %q", pr.Markdown)
	}
}

func TestGoqueryScrape_404Offline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	pr, err := newTestClient(1).Scrape(context.Background(), srv.URL+"/gone")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if pr.Failed {
		t.Fatal("404 is a page status, not an engine failure")
	}
	if pr.StatusCode != 404 {
		t.Errorf("status = %d, want 404", pr.StatusCode)
	}
}

func TestGoqueryScrape_RetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if atomic.AddInt32(&calls, 1) <= 2 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "<html><head><title>OK</title></head><body><p>recovered</p></body></html>")
	}))
	defer srv.Close()

	pr, err := newTestClient(2).Scrape(context.Background(), srv.URL+"/x")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if pr.Failed || pr.StatusCode != 200 {
		t.Fatalf("expected recovery, got failed=%v status=%d", pr.Failed, pr.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("requests = %d, want 3 (1 + 2 retries)", got)
	}
}

func TestGoqueryScrape_NetworkErrorFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close() // unreachable now

	pr, err := newTestClient(1).Scrape(context.Background(), addr+"/dead")
	if err != nil {
		t.Fatalf("expected page-level failure, got error: %v", err)
	}
	if !pr.Failed || pr.Err == "" {
		t.Fatalf("expected failed page with reason, got %+v", pr)
	}
}

func TestGoqueryScrape_RobotsBlocked(t *testing.T) {
	var hitPrivate atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/private") {
			hitPrivate.Store(true)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head><title>t</title></head><body><p>ok</p></body></html>")
	}))
	defer srv.Close()

	c := newTestClient(0)
	pr, err := c.Scrape(context.Background(), srv.URL+"/private/secret")
	if err != nil {
		t.Fatalf("Scrape error: %v", err)
	}
	if !pr.Failed || !strings.Contains(pr.Err, "robots") {
		t.Fatalf("expected robots block, got %+v", pr)
	}
	if hitPrivate.Load() {
		t.Fatal("disallowed path must never be fetched")
	}
	// Allowed path still works.
	pr2, err := c.Scrape(context.Background(), srv.URL+"/public")
	if err != nil || pr2.Failed {
		t.Fatalf("allowed path failed: %v %+v", err, pr2)
	}
}
