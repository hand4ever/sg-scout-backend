package proofread

import (
	"regexp"
	"strings"
)

// MDToPlain converts a markdown body (front-matter already stripped by the
// crawler layer) into a plain-text line stream (research D1). Line count is
// preserved 1:1 with the source so cards can anchor by line number; blank
// lines stay blank (paragraph spacing). Structure markers are removed for
// comfortable reading while ordered-list numbers and table rows are kept.
func MDToPlain(body string) []string {
	raw := strings.Split(body, "\n")
	out := make([]string, 0, len(raw))
	inFence := false
	for _, line := range raw {
		line = strings.TrimRight(line, "\r")
		if fenceRe.MatchString(line) {
			out = append(out, "")
			inFence = !inFence
			continue
		}
		if inFence {
			// fenced code content stays untouched (line break semantics).
			out = append(out, line)
			continue
		}
		out = append(out, plainLine(line))
	}
	return out
}

var (
	fenceRe  = regexp.MustCompile("^`{3,}")
	imgRe    = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	linkRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	tagRe    = regexp.MustCompile(`<[^>]*>`)
	headRe   = regexp.MustCompile(`^#{1,6}\s+`)
	quoteRe  = regexp.MustCompile(`^>+\s?`)
	bulletRe = regexp.MustCompile(`^([-*+])\s+`)
)

// plainLine strips one source line down to its readable plain text.
func plainLine(line string) string {
	// Images become an alt-text placeholder; links keep only visible text.
	line = imgRe.ReplaceAllStringFunc(line, func(s string) string {
		m := imgRe.FindStringSubmatch(s)
		if m[1] == "" {
			return ""
		}
		return "[图片:" + m[1] + "]"
	})
	line = linkRe.ReplaceAllString(line, "$1")
	line = tagRe.ReplaceAllString(line, "")
	for _, marker := range []string{"**", "__", "~~", "`"} {
		line = strings.ReplaceAll(line, marker, "")
	}
	// Leading block markers: heading symbols, blockquote arrows, list bullets
	// (ordered-list numbers are kept for revision fidelity).
	line = headRe.ReplaceAllString(line, "")
	for strings.HasPrefix(line, ">") {
		line = strings.TrimPrefix(line, ">")
		if strings.HasPrefix(line, " ") {
			line = line[1:]
		}
	}
	if bulletRe.MatchString(line) {
		line = bulletRe.ReplaceAllString(line, "")
	}
	// Leftover single emphasis markers (rare in CJK article bodies).
	if strings.ContainsAny(line, "*_") {
		line = strings.Map(func(r rune) rune {
			if r == '*' || r == '_' {
				return -1
			}
			return r
		}, line)
	}
	return line
}
