package engine

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
)

// ExtractMainMarkdown runs go-readability (Mozilla Readability port, MIT) over
// a full HTML page and returns the extracted title + body markdown. It powers
// task content_mode="main" (article-only archiving): nav/sidebars/footers are
// dropped so stored markdown is the page's main content (feature 002).
// ok=false when extraction fails or yields no content — callers keep the
// original full-page markdown then.
func ExtractMainMarkdown(rawHTML, rawURL string) (title, md string, ok bool) {
	if strings.TrimSpace(rawHTML) == "" {
		return "", "", false
	}
	u, _ := url.Parse(rawURL)
	article, err := readability.FromReader(strings.NewReader(rawHTML), u)
	if err != nil {
		return "", "", false
	}
	if strings.TrimSpace(article.Content) == "" {
		return "", "", false
	}
	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(article.Content))
	if derr != nil {
		return "", "", false
	}
	return strings.TrimSpace(article.Title), htmlToMarkdown(doc), true
}
