package proofread

import (
	"fmt"
	"os"
	"strings"
)

// Lexicon engine support (research D7): a user-maintained text file mapping
// wrong words to corrections, one rule per line:
//
//	# comment
//	错词→正词
//	错词=>正词
//	错词<TAB>正词
//
// Matching is exact (rune-level) over the draft; a word's first occurrence in
// the document yields one candidate (research D4/D11).

// lexiconEntry is one parsed rule.
type lexiconEntry struct {
	Word        string // the wrong word to find
	Replacement string // the correction
}

// ParseLexicon parses dictionary file content into ordered entries. Lines
// starting with '#' or blank are skipped; unparsable lines return an error
// naming the line so the user can fix the file.
func ParseLexicon(content string) ([]lexiconEntry, error) {
	var out []lexiconEntry
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sep := "→"
		if strings.Contains(line, "=>") {
			sep = "=>"
		} else if strings.Contains(line, "\t") {
			sep = "\t"
		}
		parts := strings.SplitN(line, sep, 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("词库第 %d 行无法解析（期望格式：错词→正词）：%q", i+1, line)
		}
		word := strings.TrimSpace(parts[0])
		repl := strings.TrimSpace(parts[1])
		if word == "" || repl == "" {
			return nil, fmt.Errorf("词库第 %d 行错词或正词为空：%q", i+1, line)
		}
		out = append(out, lexiconEntry{Word: word, Replacement: repl})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("词库不含任何有效词条")
	}
	return out, nil
}

// LexiconMatch is one dictionary hit anchored into the draft.
type LexiconMatch struct {
	Entry     lexiconEntry
	StartLine int
	StartOff  int
	EndLine   int
	EndOff    int
}

// MatchLexicon scans the draft line stream for every lexicon word. Only the
// first occurrence of each word is matched (research D11); the caller turns
// hits into candidates.
func MatchLexicon(lines []string, entries []lexiconEntry) []LexiconMatch {
	var out []LexiconMatch
	for _, e := range entries {
		if a, ok := findInLines(lines, e.Word); ok {
			out = append(out, LexiconMatch{Entry: e, StartLine: a.StartLine,
				StartOff: a.StartOff, EndLine: a.EndLine, EndOff: a.EndOff})
		}
	}
	return out
}

// findInLines searches word inside the joined line stream (first match).
func findInLines(lines []string, word string) (cardAnchor, bool) {
	draft := strings.Join(lines, "\n")
	return Locate(draft, word)
}

// LoadLexiconFile reads and parses a dictionary file (research D7: the file is
// the single source of truth; each run re-reads it so edits take effect).
func LoadLexiconFile(path string) ([]lexiconEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("词库文件读取失败（%s）：%v", path, err)
	}
	return ParseLexicon(string(b))
}
