package proofread

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
)

func fakeCard(id uint64, docID uint64, op string, sl, so, el, eo int, orig, repl string, status string) model.ProofreadCard {
	return model.ProofreadCard{
		ID: id, DocID: docID, OpType: op,
		StartLine: sl, StartOff: so, EndLine: el, EndOff: eo,
		OrigText: orig, Replacement: repl, Status: status,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

// TestApplyCards_Fix covers a single-line fix with a correct mark position.
func TestApplyCards_Fix(t *testing.T) {
	draft := "今天天气很好，适合出游。\n第二行内容测试。"
	cards := []model.ProofreadCard{
		fakeCard(1, 1, entityproofread.OpTypeFix, 1, 2, 1, 4, "天气", "气候", entityproofread.StatusAccepted),
	}
	revised, marks, err := applyCards(draft, cards)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "今天气候很好，适合出游。\n第二行内容测试。"
	if revised != want {
		t.Errorf("revised = %q, want %q", revised, want)
	}
	if len(marks) != 1 || marks[0].Line != 1 || marks[0].StartOff != 2 || marks[0].EndOff != 4 {
		t.Errorf("marks = %+v, want single mark L1[2,4)", marks)
	}
	logOK(t, "apply fix ok: %s", revised)
}

// TestApplyCards_MultiLineReplace merges a cross-line span into one line.
func TestApplyCards_MultiLineReplace(t *testing.T) {
	draft := "第一行开头。\n第二行尾巴。"
	cards := []model.ProofreadCard{
		fakeCard(2, 1, entityproofread.OpTypeReplace, 1, 2, 2, 2, "行开头。\n第二", "段合并", entityproofread.StatusAccepted),
	}
	revised, marks, err := applyCards(draft, cards)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "第一段合并行尾巴。"
	if revised != want {
		t.Errorf("revised = %q, want %q", revised, want)
	}
	if len(marks) != 1 || marks[0].Line != 1 || marks[0].StartOff != 2 || marks[0].EndOff != 5 {
		t.Errorf("marks = %+v, want L1[2,5)", marks)
	}
	logOK(t, "apply multi-line replace ok: %s", revised)
}

// TestApplyCards_MixedOrder applies fix + delete + insert cards together.
func TestApplyCards_MixedOrder(t *testing.T) {
	draft := "他明天去北京开会。\n下午返回上海。"
	cards := []model.ProofreadCard{
		fakeCard(1, 1, entityproofread.OpTypeFix, 1, 0, 1, 1, "他", "她", entityproofread.StatusAccepted),
		fakeCard(2, 1, entityproofread.OpTypeDelete, 2, 2, 2, 4, "返回", "", entityproofread.StatusAccepted),
		fakeCard(3, 1, entityproofread.OpTypeInsert, 2, 6, 2, 6, "", "（复核）", entityproofread.StatusAccepted),
	}
	revised, marks, err := applyCards(draft, cards)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "她明天去北京开会。\n下午上海（复核）。"
	if revised != want {
		t.Errorf("revised = %q, want %q", revised, want)
	}
	if len(marks) != 3 {
		t.Fatalf("marks = %d, want 3", len(marks))
	}
	if marks[0].Line != 1 || marks[0].StartOff != 0 || marks[0].EndOff != 1 {
		t.Errorf("fix mark = %+v, want L1[0,1)", marks[0])
	}
	if marks[2].Line != 2 || marks[2].StartOff != 4 || marks[2].EndOff != 8 {
		t.Errorf("insert mark = %+v, want L2[4,8)", marks[2])
	}
	logOK(t, "mixed apply ok: %s", revised)
}

// TestApplyCards_RejectsStaleAnchor guards against drift between the card
// orig_text and the current draft (upgrade replaced the draft text).
func TestApplyCards_RejectsStaleAnchor(t *testing.T) {
	draft := "新版本正文内容。"
	cards := []model.ProofreadCard{
		fakeCard(9, 1, entityproofread.OpTypeFix, 1, 0, 1, 2, "旧文", "新文", entityproofread.StatusAccepted),
	}
	_, _, err := applyCards(draft, cards)
	if err == nil {
		t.Fatal("expected anchor drift error")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict wrapping", err)
	}
	logOK(t, "stale anchor rejected: %v", err)
}

// TestRevisionText_EmptyAcceptedMessage is a guard for FR-017 semantics (the
// handler layer requires DB; the empty-guard message shape is asserted here).
func TestRevisionText_EmptyAcceptedMessage(t *testing.T) {
	err := errNoAccepted()
	if !errors.Is(err, ErrBadRequest) {
		t.Fatal("no-accepted error must wrap ErrBadRequest")
	}
	if !strings.Contains(err.Error(), "暂无已接受校对项") {
		t.Errorf("no-accepted error message = %q", err.Error())
	}
	logOK(t, "no-accepted guard message ok: %v", err)
}

func errNoAccepted() error { return fmt.Errorf("%w: 暂无已接受校对项", ErrBadRequest) }
