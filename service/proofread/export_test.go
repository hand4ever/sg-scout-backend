package proofread

import (
	"bytes"
	"strings"
	"testing"
	"time"

	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
)

// TestErrataCSVBytes checks BOM, header, accepted-only rows, sequential
// numbering and the 中文 type mapping.
func TestErrataCSVBytes(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 30, 0, 0, time.Local)
	doc := &model.ProofreadDocument{DraftText: "第一行有问题文字。\n第二行正常。"}
	cards := []model.ProofreadCard{
		{ID: 1, OpType: entityproofread.OpTypeFix, StartLine: 1, Status: entityproofread.StatusAccepted,
			OrigText: "问题", Replacement: "问题已改", Reason: "错别字", UpdatedAt: now},
		{ID: 2, OpType: entityproofread.OpTypeDelete, StartLine: 2, Status: entityproofread.StatusRejected,
			OrigText: "正常", Reason: "不必删"},
		{ID: 3, OpType: entityproofread.OpTypeInsert, StartLine: 2, Status: entityproofread.StatusAccepted,
			Replacement: "（增补）", Reason: "补充说明", UpdatedAt: now},
	}
	data, err := ErrataCSVBytes(doc, cards)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\ufeff")) {
		t.Error("missing UTF-8 BOM")
	}
	body := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) != 3 { // header + 2 accepted rows
		t.Fatalf("rows = %d, want 3 (header + 2 accepted)", len(lines))
	}
	if lines[0] != "序号,位置,类型,原文,拟改,理由,校对时间" {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "改正") || !strings.Contains(lines[1], "L1") {
		t.Errorf("row1 = %q, want 改正 + L1", lines[1])
	}
	if !strings.HasPrefix(lines[2], "2,") || !strings.Contains(lines[2], "增补") {
		t.Errorf("row2 = %q, want seq 2 + 增补 (rejected skipped)", lines[2])
	}
	if strings.Contains(body, "删除") && strings.Contains(lines[2], "删除") {
		t.Errorf("rejected delete leaked: %q", lines[2])
	}
	logOK(t, "errata csv ok: %d rows", len(lines)-1)
}

// TestErrataPosition_Truncate covers the ≤20-rune context truncation.
func TestErrataPosition_Truncate(t *testing.T) {
	long := strings.Repeat("字", 30)
	lines := draftLines(long + "\nb")
	card := &model.ProofreadCard{StartLine: 1}
	pos := errataPosition(card, lines)
	if !strings.HasPrefix(pos, "L1·") {
		t.Errorf("pos = %q, want L1· prefix", pos)
	}
	ctx := strings.TrimPrefix(pos, "L1·")
	if len([]rune(ctx)) != 20 {
		t.Errorf("context runes = %d, want 20", len([]rune(ctx)))
	}
	logOK(t, "position truncation ok: %s", pos)
}
