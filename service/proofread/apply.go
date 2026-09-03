package proofread

import (
	"fmt"
	"strings"
	"time"

	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
)

// RevisionMarkView locates one applied change inside the REVISED text
// (preview-only highlight; the exported file stays clean — spec Q8).
type RevisionMarkView struct {
	CardID   uint64 `json:"card_id"`
	OpType   string `json:"op_type"`
	Line     int    `json:"line"`
	StartOff int    `json:"start_off"`
	EndOff   int    `json:"end_off"`
	Reason   string `json:"reason"`
}

// RevisionView is GET /proofreads/{id}/revision payload (contracts §11).
type RevisionView struct {
	DraftVersion *int               `json:"draft_version"`
	Revised      string             `json:"revised"`
	Marks        []RevisionMarkView `json:"marks"`
}

// RevisionPreview applies every accepted card to the draft and returns the
// revised full text plus per-change marks (research D4). No accepted cards →
// ErrBadRequest「暂无已接受校对项」(FR-017).
func RevisionPreview(docID uint64) (*RevisionView, error) {
	doc, err := docByID(docID)
	if err != nil {
		return nil, err
	}
	cards, err := acceptedCards(doc.ID)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("%w: 暂无已接受校对项", ErrBadRequest)
	}
	revised, marks, err := applyCards(doc.DraftText, cards)
	if err != nil {
		return nil, err
	}
	return &RevisionView{DraftVersion: doc.DraftVersion, Revised: revised, Marks: marks}, nil
}

// RevisionText returns the clean revised full text for export (Q8: no marks).
func RevisionText(docID uint64) (string, error) {
	doc, err := docByID(docID)
	if err != nil {
		return "", err
	}
	cards, err := acceptedCards(doc.ID)
	if err != nil {
		return "", err
	}
	if len(cards) == 0 {
		return "", fmt.Errorf("%w: 暂无已接受校对项", ErrBadRequest)
	}
	revised, _, err := applyCards(doc.DraftText, cards)
	return revised, err
}

// DeriveRevisionDoc creates a child proofread document whose draft is this
// document's revision (FR-021 / Q5). The chain generation is recorded in the
// title: "«parent title»·修订N" where N = parent generation + 1.
func DeriveRevisionDoc(docID uint64) (*DocView, error) {
	doc, err := docByID(docID)
	if err != nil {
		return nil, err
	}
	cards, err := acceptedCards(doc.ID)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("%w: 暂无已接受校对项", ErrBadRequest)
	}
	revised, _, err := applyCards(doc.DraftText, cards)
	if err != nil {
		return nil, err
	}
	gen := generationOf(doc) + 1
	title := "修订" + itoaUint(uint64(gen))
	if doc.Title != "" {
		title = doc.Title + "·修订" + itoaUint(uint64(gen))
	}
	now := time.Now()
	child := model.ProofreadDocument{
		SourceType:  entityproofread.SourceRevision,
		ParentDocID: &doc.ID,
		Title:       title,
		SourceURL:   doc.SourceURL,
		DraftText:   revised,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := model.DB.Create(&child).Error; err != nil {
		return nil, err
	}
	return docViewOf(&child), nil
}

// acceptedCards loads all accepted cards of a doc, ascending by anchor.
func acceptedCards(docID uint64) ([]model.ProofreadCard, error) {
	var cards []model.ProofreadCard
	if err := model.DB.Where("doc_id = ? AND status = ?", docID, entityproofread.StatusAccepted).
		Order("start_line ASC, start_off ASC, id ASC").Find(&cards).Error; err != nil {
		return nil, err
	}
	if cards == nil {
		cards = []model.ProofreadCard{}
	}
	return cards, nil
}

// applyCards merges accepted cards into the draft. Cards are pairwise disjoint
// (FR-006 overlap ban), so the merge is deterministic: assemble ascending by
// original coordinates — each gap piece and replacement appended once, in
// order (research D4). Mark line/offsets are computed against the assembled
// (revised) text with incremental counters.
func applyCards(draft string, cards []model.ProofreadCard) (string, []RevisionMarkView, error) {
	lines := draftLines(draft)
	flat := []rune(strings.Join(lines, "\n"))
	lineStart := make([]int, len(lines)+1)
	acc := 0
	for i, ln := range lines {
		lineStart[i] = acc
		acc += len([]rune(ln)) + 1 // +1 for the joining '\n'
	}
	lineStart[len(lines)] = acc

	marks := []RevisionMarkView{}
	var b strings.Builder
	cursor := 0
	curLine := 1 // 1-based revised line of the next rune to be appended
	curCol := 0  // rune column within curLine
	appendPiece := func(s string) {
		b.WriteString(s)
		nl := strings.Count(s, "\n")
		if nl == 0 {
			curCol += len([]rune(s))
			return
		}
		// position after the last '\n'
		tail := s[strings.LastIndex(s, "\n")+1:]
		curLine += nl
		curCol = len([]rune(tail))
	}
	for i := range cards {
		card := &cards[i]
		startIdx := lineStart[card.StartLine-1] + card.StartOff
		endIdx := lineStart[card.EndLine-1] + card.EndOff
		if startIdx < cursor || endIdx < startIdx || endIdx > len(flat) {
			return "", nil, fmt.Errorf("%w: 卡片 L%d 锚点与底稿不一致，请删除后重建", ErrConflict, card.StartLine)
		}
		got := string(flat[startIdx:endIdx])
		if got != card.OrigText {
			return "", nil, fmt.Errorf("%w: 卡片 L%d 原文与底稿不一致，请删除后重建", ErrConflict, card.StartLine)
		}
		appendPiece(string(flat[cursor:startIdx]))
		repl := card.Replacement
		if card.OpType == entityproofread.OpTypeDelete {
			repl = ""
		}
		startLine, startOff := curLine, curCol
		appendPiece(repl)
		marks = append(marks, RevisionMarkView{
			CardID: card.ID, OpType: card.OpType, Line: startLine,
			StartOff: startOff, EndOff: curCol, Reason: card.Reason,
		})
		cursor = endIdx
	}
	appendPiece(string(flat[cursor:]))
	return b.String(), marks, nil
}

// generationOf walks the parent chain to compute a doc's generation (base=1).
func generationOf(doc *model.ProofreadDocument) int {
	gen := 1
	id := doc.ParentDocID
	for i := 0; i < 100 && id != nil; i++ {
		var p model.ProofreadDocument
		if err := model.DB.First(&p, *id).Error; err != nil {
			break
		}
		gen++
		id = p.ParentDocID
	}
	return gen
}

// itoaUint formats a uint64 without strconv ceremony at call sites.
func itoaUint(v uint64) string { return fmt.Sprintf("%d", v) }
