package engine

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// excludedTags never contribute visible text (script/style keep the markdown
// clean; head/iframe/svg are not readable content).
var excludedTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true,
	"head": true, "iframe": true, "svg": true, "audio": true, "video": true,
	"canvas": true, "map": true,
}

// blockTags are structural blocks: content is followed by a newline.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "header": true,
	"footer": true, "nav": true, "main": true, "aside": true, "ul": true,
	"ol": true, "table": true, "tr": true, "blockquote": true, "figure": true,
	"pre": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "li": true, "td": true, "th": true, "form": true, "address": true,
}

// htmlToMarkdown converts the full visible page (nav/footer included — 001 A5
// fingerprint scope) into lightweight markdown: headings, links and list
// items are preserved; cross-engine formatting differences are acceptable
// (feature 002 A3).
func htmlToMarkdown(doc *goquery.Document) string {
	root := doc.Selection
	if body := doc.Find("body"); body.Length() > 0 {
		root = body
	}
	var b strings.Builder
	if n := root.Get(0); n != nil {
		walkHTML(&b, n, excludedTags)
	}
	return strings.TrimSpace(b.String())
}

func walkHTML(b *strings.Builder, n *html.Node, skip map[string]bool) {
	walkHTMLInner(b, n, skip, false)
}

// walkHTMLInner walks the node tree writing markdown. When preserve is true
// (inside <pre>) text is kept verbatim so code blocks survive; otherwise text
// node whitespace is collapsed — source newlines/indentation from formatted
// HTML must not leak into the markdown as stray blank lines.
func walkHTMLInner(b *strings.Builder, n *html.Node, skip map[string]bool, preserve bool) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.TextNode:
		if preserve {
			t := n.Data
			if t != "" {
				b.WriteString(t)
				if !strings.HasSuffix(t, " ") && !strings.HasSuffix(t, "\n") {
					b.WriteString(" ")
				}
			}
			return
		}
		// Collapse all whitespace runs (incl. source newlines) to one space;
		// whitespace-only nodes vanish entirely.
		t := strings.Join(strings.Fields(n.Data), " ")
		if t != "" {
			b.WriteString(t)
			if !strings.HasSuffix(t, " ") {
				b.WriteString(" ")
			}
		}
		return
	case html.ElementNode:
		tag := n.Data
		if skip[tag] {
			return
		}
		switch tag {
		case "br":
			b.WriteString("\n")
			return
		case "img":
			for _, a := range n.Attr {
				if a.Key == "alt" && strings.TrimSpace(a.Val) != "" {
					b.WriteString(strings.TrimSpace(a.Val) + " ")
				}
			}
			return
		case "a":
			href := ""
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = a.Val
				}
			}
			if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") ||
				strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walkHTMLInner(b, c, skip, preserve)
				}
				return
			}
			var inner strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walkHTMLInner(&inner, c, skip, preserve)
			}
			text := strings.TrimSpace(inner.String())
			if text == "" {
				return
			}
			b.WriteString("[" + text + "](" + href + ") ")
			return
		case "pre":
			// Keep <pre> content verbatim (code semantics: newlines matter).
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walkHTMLInner(b, c, skip, true)
			}
			b.WriteString("\n")
			return
		}
		// Structural prefix for list items / headings.
		switch {
		case tag == "li":
			b.WriteString("\n- ")
		case len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6':
			b.WriteString("\n" + strings.Repeat("#", int(tag[1]-'0')) + " ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkHTMLInner(b, c, skip, preserve)
		}
		if blockTags[tag] {
			b.WriteString("\n")
		}
		return
	}
}
