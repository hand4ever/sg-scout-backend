package proofread

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
)

// ErrataCSVBytes renders the errata sheet (UTF-8 BOM + CSV, contracts §11).
// Only ACCEPTED cards are exported (spec Q3 / FR-016). Columns follow FR-016:
// 序号 / 位置 / 类型 / 原文 / 拟改 / 理由 / 校对时间.
func ErrataCSVBytes(doc *model.ProofreadDocument, cards []model.ProofreadCard) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff") // UTF-8 BOM so Excel/WPS detect UTF-8 (SC-002)
	w := csv.NewWriter(&buf)
	header := []string{"序号", "位置", "类型", "原文", "拟改", "理由", "校对时间"}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	lines := draftLines(doc.DraftText)
	seq := 1
	for i := range cards {
		card := &cards[i]
		if card.Status != entityproofread.StatusAccepted {
			continue
		}
		pos := errataPosition(card, lines)
		row := []string{
			fmt.Sprintf("%d", seq),
			pos,
			opLabelCN(card.OpType),
			card.OrigText,
			card.Replacement,
			card.Reason,
			card.UpdatedAt.Format("2006-01-02 15:04"),
		}
		seq++
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// ErrataExport loads the doc, keeps only accepted cards and renders the CSV
// (FR-016/017). Zero accepted cards → ErrBadRequest (no empty artifacts).
func ErrataExport(docID uint64) ([]byte, error) {
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
	return ErrataCSVBytes(doc, cards)
}

// errataPosition renders "L{line}·{≤20 字行首摘录}" for the errata sheet.
func errataPosition(card *model.ProofreadCard, lines []string) string {
	pos := fmt.Sprintf("L%d", card.StartLine)
	if card.StartLine >= 1 && card.StartLine <= len(lines) {
		ctx := strings.TrimSpace(lines[card.StartLine-1])
		r := []rune(ctx)
		if len(r) > 20 {
			r = r[:20]
		}
		if len(r) > 0 {
			pos += "·" + string(r)
		}
	}
	return pos
}
