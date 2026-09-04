package proofread

import (
	"fmt"
	"strings"
	"unicode/utf8"

	entityproofread "sg.scout/entity/proofread"
)

// Engine candidate types (research D4): engines return text-anchored proposals;
// the server locates them inside the draft line stream. Insert proposals are
// NOT accepted from engines in v1 (anchoring an insertion point from model
// output is unreliable; users add insertions manually in the workbench).
var engineOpTypes = map[string]bool{
	entityproofread.OpTypeFix:     true,
	entityproofread.OpTypeReplace: true,
	entityproofread.OpTypeDelete:  true,
}

// Candidate is one engine proposal before it becomes a card (research D4).
// No line/offset fields: the server locates OrigText precisely.
type Candidate struct {
	OpType      string
	OrigText    string
	Replacement string
	Reason      string
}

// ValidateCandidate checks type rules and replacement requirements shared by
// every engine (same semantics as FR-006 for fix/replace/delete).
func ValidateCandidate(c Candidate) error {
	if !engineOpTypes[c.OpType] {
		return fmt.Errorf("引擎候选仅支持 改正/替换/删除（%s 不受支持）", c.OpType)
	}
	if c.OpType != entityproofread.OpTypeDelete && strings.TrimSpace(c.Replacement) == "" {
		return fmt.Errorf("%s 候选缺少拟改内容", opLabelCN(c.OpType))
	}
	if c.OrigText == "" {
		return fmt.Errorf("%s 候选缺少原文", opLabelCN(c.OpType))
	}
	if runeLen(c.Replacement) > entityproofread.MaxCardFieldLen {
		return fmt.Errorf("拟改内容超长（上限 %d 字符）", entityproofread.MaxCardFieldLen)
	}
	if strings.ContainsRune(c.Replacement, '\n') {
		return fmt.Errorf("拟改内容不支持换行")
	}
	if runeLen(c.Reason) > entityproofread.MaxCardFieldLen {
		return fmt.Errorf("理由超长（上限 %d 字符）", entityproofread.MaxCardFieldLen)
	}
	return nil
}

// Locate finds OrigText inside the draft (plain line stream, lines joined by
// \n) and returns the anchored interval, first match wins (research D4).
// Multi-line originals span lines and keep embedded \n.
func Locate(draft, orig string) (cardAnchor, bool) {
	if orig == "" {
		return cardAnchor{}, false
	}
	byteIdx := strings.Index(draft, orig)
	if byteIdx < 0 {
		return cardAnchor{}, false
	}
	startL, startO := anchorFromPrefix(draft, byteIdx)
	endL, endO := anchorFromPrefix(draft, byteIdx+len(orig))
	if endL == startL && endO == startO {
		return cardAnchor{}, false // degenerate: zero-length match impossible for non-empty orig
	}
	return cardAnchor{StartLine: startL, StartOff: startO,
		EndLine: endL, EndOff: endO}, true
}

// anchorFromPrefix converts a byte offset in draft into a (line, rune-offset)
// position: line = 1-based count of '\n' before the offset + 1; offset = rune
// count after the last '\n' before the offset.
func anchorFromPrefix(draft string, byteIdx int) (line, off int) {
	prefix := draft[:byteIdx]
	line = strings.Count(prefix, "\n") + 1
	lastNL := strings.LastIndexByte(prefix, '\n')
	seg := prefix[lastNL+1:]
	return line, utf8.RuneCountInString(seg)
}

// ExtractOrigByAnchor slices draft text for the anchor interval (used to build
// the persisted orig_text snapshot from the located interval).
func ExtractOrigByAnchor(draft string, a cardAnchor) (string, bool) {
	lines := draftLines(draft)
	if a.StartLine < 1 || a.EndLine < a.StartLine || a.EndLine > len(lines) {
		return "", false
	}
	orig, err := extractOrig(lines, a)
	if err != nil {
		return "", false
	}
	return orig, true
}
