package engine

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// htmlToMarkdownSource renders the given html source (formatted with newlines
// like a real page) through htmlToMarkdown.
func htmlToMarkdownSource(t *testing.T, html string) string {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return htmlToMarkdown(doc)
}

// Regression (user report 2026-09-04): formatted HTML source newlines leaked
// into the markdown as stray blank lines/line breaks.
func TestHTMLMD_CollapsesSourceNewlines(t *testing.T) {
	src := "<html><body><main>\n  <h1>标题\n</h1>\n  <p>\n    第一段\n    内容继续。\n  </p>\n  <p>第二段\n简洁内容。</p>\n</main></body></html>"
	md := htmlToMarkdownSource(t, src)
	if strings.Contains(md, "\n\n\n") {
		t.Errorf("blank runs must collapse, got:\n%s", md)
	}
	if !strings.Contains(md, "# 标题") || !strings.Contains(md, "第一段 内容继续。") {
		t.Errorf("content missing after collapse:\n%s", md)
	}
	// 段落间应只有一个空行分隔（块级换行），无源码空白行。
	for _, line := range strings.Split(md, "\n") {
		if strings.TrimSpace(line) == "" {
			t.Errorf("no whitespace-only lines expected:\n%q", md)
		}
	}
}

// Code blocks must survive verbatim (newlines inside <pre> are semantic).
func TestHTMLMD_PreservesPre(t *testing.T) {
	src := "<html><body><main><p>说明</p><pre>\nline1\n  line2\n</pre><p>结尾</p></main></body></html>"
	md := htmlToMarkdownSource(t, src)
	if !strings.Contains(md, "line1\n  line2") {
		t.Errorf("pre content must stay verbatim:\n%s", md)
	}
}

// Inline text inside one paragraph with formatted source must join with a
// single space, not wrap mid-sentence.
func TestHTMLMD_InlineJoin(t *testing.T) {
	src := "<html><body><main><p>据\n<span>介绍</span>，\n该院\n已开展</p></main></body></html>"
	md := htmlToMarkdownSource(t, src)
	joined := strings.Join(strings.Fields(md), " ")
	if !strings.Contains(joined, "据 介绍 ， 该院 已开展") {
		t.Errorf("inline fragments should join on one line, got: %s", md)
	}
}
