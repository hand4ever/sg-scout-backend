package crawler

import (
	"testing"
)

func TestNormalizeURL_TrackingParams(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "utm and share params dropped",
			in:   "https://Example.com/a?utm_source=x&id=1&from=wechat",
			want: "https://example.com/a?id=1",
		},
		{
			name: "www prefix stripped",
			in:   "https://www.example.com/a",
			want: "https://example.com/a",
		},
		{
			name: "trailing slash dropped",
			in:   "http://example.com/a/",
			want: "http://example.com/a",
		},
		{
			name: "root slash kept",
			in:   "http://example.com",
			want: "http://example.com/",
		},
		{
			name: "fragment dropped",
			in:   "https://example.com/a#sec",
			want: "https://example.com/a",
		},
		{
			name: "query sorted",
			in:   "https://example.com/a?b=2&a=1",
			want: "https://example.com/a?a=1&b=2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeURL(c.in)
			if err != nil {
				t.Fatalf("normalizeURL(%q) error: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("expected %q, got %q", c.want, got)
			}
		})
	}
}

func TestNormalizeURL_UnsupportedScheme(t *testing.T) {
	if _, err := normalizeURL("ftp://example.com/a"); err == nil {
		t.Error("expected error for ftp scheme, got nil")
	}
}

func TestURLKey_DedupEquivalence(t *testing.T) {
	a, _ := URLKey("https://www.Example.com/news?utm_source=x&id=5")
	b, _ := URLKey("https://example.com/news?id=5#top")
	if a != b {
		t.Errorf("expected same key for equivalent URLs, got %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(a))
	}
}

func TestSameHost(t *testing.T) {
	if !SameHost("example.com", "www.example.com") {
		t.Error("www variant should be same host")
	}
	if !SameHost("WWW.Example.com", "example.com") {
		t.Error("case + www variant should be same host")
	}
	if SameHost("example.com", "news.example.com") {
		t.Error("subdomain must NOT be same host by default")
	}
	if SameHost("example.com", "other.org") {
		t.Error("different domain must not be same host")
	}
}

func TestSubdomainAllowed(t *testing.T) {
	if !SubdomainAllowed("example.com", "news.example.com") {
		t.Error("subdomain should be allowed when include_subdomain on")
	}
	if !SubdomainAllowed("example.com", "example.com") {
		t.Error("bare host should be allowed")
	}
	if SubdomainAllowed("example.com", "example.org") || SubdomainAllowed("example.com", "notexample.com") {
		t.Error("unrelated domains must be rejected")
	}
}

func TestIsNonWebURL(t *testing.T) {
	nonWeb := []string{"https://example.com/a.pdf", "https://example.com/x/image.JPG?w=1", "https://example.com/v.mp4"}
	for _, u := range nonWeb {
		if !IsNonWebURL(u) {
			t.Errorf("expected %q flagged as non-web", u)
		}
	}
	web := []string{"https://example.com/a", "https://example.com/a.html", "https://example.com/x?id=1"}
	for _, u := range web {
		if IsNonWebURL(u) {
			t.Errorf("expected %q treated as web page", u)
		}
	}
}

func TestFingerprint_StableAndSensitive(t *testing.T) {
	a := Fingerprint("hello world")
	b := Fingerprint("hello world")
	if a != b {
		t.Error("fingerprint must be stable")
	}
	if a == Fingerprint("hello world!") {
		t.Error("fingerprint must change with content")
	}
	if len(a) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(a))
	}
}
