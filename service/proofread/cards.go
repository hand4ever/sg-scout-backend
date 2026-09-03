package proofread

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
)

// cardAnchor is the validated interval of a card inside the draft line stream.
type cardAnchor struct {
	StartLine int
	StartOff  int
	EndLine   int
	EndOff    int
}

// CreateCard validates and persists a new proofreading card (FR-004~006).
// The anchored original text is extracted server-side from the draft, so the
// client cannot forge it (research D2). Overlapping cards are rejected.
func CreateCard(docID uint64, req *entityproofread.CardCreateReq) (*model.ProofreadCard, error) {
	doc, err := docByID(docID)
	if err != nil {
		return nil, err
	}
	if !validOpType(req.OpType) {
		return nil, fmt.Errorf("%w: 无效的操作类型", ErrBadRequest)
	}
	lines := draftLines(doc.DraftText)
	anchor, origText, err := resolveAnchor(lines, req.StartLine, req.StartOff, req.EndLine, req.EndOff, req.OpType)
	if err != nil {
		return nil, err
	}
	if runeLen(req.Replacement) > entityproofread.MaxCardFieldLen {
		return nil, fmt.Errorf("%w: 拟改内容超长（上限 %d 字符）", ErrBadRequest, entityproofread.MaxCardFieldLen)
	}
	if strings.ContainsRune(req.Replacement, '\n') {
		return nil, fmt.Errorf("%w: 拟改内容不支持换行，请分行校对", ErrBadRequest)
	}
	if runeLen(req.Reason) > entityproofread.MaxCardFieldLen {
		return nil, fmt.Errorf("%w: 理由超长（上限 %d 字符）", ErrBadRequest, entityproofread.MaxCardFieldLen)
	}
	// Overlap guard (FR-006): reject any intersection with existing cards,
	// including an insert point landing inside another card's range.
	var existing []model.ProofreadCard
	if err := model.DB.Where("doc_id = ?", docID).
		Order("start_line ASC, start_off ASC, id ASC").Find(&existing).Error; err != nil {
		return nil, err
	}
	for i := range existing {
		e := &existing[i]
		if anchorsOverlap(anchorOf(e), cardAnchor{req.StartLine, req.StartOff, req.EndLine, req.EndOff}) {
			return nil, fmt.Errorf("%w: 与既有校对项区域重叠（L%d）", ErrBadRequest, e.StartLine)
		}
	}
	replacement := req.Replacement
	if req.OpType == entityproofread.OpTypeDelete {
		replacement = "" // delete carries no replacement (FR-005)
	}
	if (req.OpType == entityproofread.OpTypeFix || req.OpType == entityproofread.OpTypeReplace ||
		req.OpType == entityproofread.OpTypeInsert) && strings.TrimSpace(replacement) == "" {
		return nil, fmt.Errorf("%w: %s操作必填拟改内容", ErrBadRequest, opLabelCN(req.OpType))
	}
	now := time.Now()
	card := model.ProofreadCard{
		DocID:         docID,
		OpType:        req.OpType,
		StartLine:     anchor.StartLine,
		StartOff:      anchor.StartOff,
		EndLine:       anchor.EndLine,
		EndOff:        anchor.EndOff,
		OrigText:      origText,
		Replacement:   replacement,
		Reason:        strings.TrimSpace(req.Reason),
		Status:        entityproofread.StatusPending,
		AnchorVersion: doc.DraftVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := model.DB.Create(&card).Error; err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("L%d 新建卡片：%s%s", card.StartLine, opLabelCN(card.OpType), briefDiff(&card))
	if err := writeLog(docID, logActionCardCreate, &card.ID, summary, map[string]any{
		"op_type": card.OpType, "start_line": card.StartLine,
	}); err != nil {
		return nil, err
	}
	return &card, nil
}

// UpdateCard edits card fields (type/replacement/reason). Field edits never
// change the status (spec Q6 / FR-008).
func UpdateCard(docID, cardID uint64, req *entityproofread.CardUpdateReq) (*model.ProofreadCard, error) {
	card, err := cardByID(docID, cardID)
	if err != nil {
		return nil, err
	}
	before := cardSummaryOf(card)
	if req.OpType != nil {
		if !validOpType(*req.OpType) {
			return nil, fmt.Errorf("%w: 无效的操作类型", ErrBadRequest)
		}
		card.OpType = *req.OpType
	}
	if req.Replacement != nil {
		if runeLen(*req.Replacement) > entityproofread.MaxCardFieldLen {
			return nil, fmt.Errorf("%w: 拟改内容超长（上限 %d 字符）", ErrBadRequest, entityproofread.MaxCardFieldLen)
		}
		if strings.ContainsRune(*req.Replacement, '\n') {
			return nil, fmt.Errorf("%w: 拟改内容不支持换行，请分行校对", ErrBadRequest)
		}
		card.Replacement = *req.Replacement
	}
	if req.Reason != nil {
		if runeLen(*req.Reason) > entityproofread.MaxCardFieldLen {
			return nil, fmt.Errorf("%w: 理由超长（上限 %d 字符）", ErrBadRequest, entityproofread.MaxCardFieldLen)
		}
		card.Reason = strings.TrimSpace(*req.Reason)
	}
	// Re-validate against the type rules (FR-005/006). An insert card is an
	// empty point; non-insert cards must carry a non-empty anchored range.
	if card.OpType == entityproofread.OpTypeInsert {
		if card.StartLine != card.EndLine || card.StartOff != card.EndOff {
			return nil, fmt.Errorf("%w: 增补卡片必须是空点；原区间非空请删除后按类型重建", ErrBadRequest)
		}
	} else if card.StartLine == card.EndLine && card.StartOff == card.EndOff {
		return nil, fmt.Errorf("%w: 空点区间仅支持增补操作", ErrBadRequest)
	}
	if card.OpType == entityproofread.OpTypeDelete {
		card.Replacement = ""
	}
	if (card.OpType == entityproofread.OpTypeFix || card.OpType == entityproofread.OpTypeReplace ||
		card.OpType == entityproofread.OpTypeInsert) && strings.TrimSpace(card.Replacement) == "" {
		return nil, fmt.Errorf("%w: %s操作必填拟改内容", ErrBadRequest, opLabelCN(card.OpType))
	}
	card.UpdatedAt = time.Now()
	if err := model.DB.Save(&card).Error; err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("L%d 编辑卡片（%d）", card.StartLine, card.ID)
	if err := writeLog(docID, logActionCardUpdate, &card.ID, summary, map[string]any{
		"before": before, "after": cardSummaryOf(card),
	}); err != nil {
		return nil, err
	}
	return card, nil
}

// DeleteCard removes a card; the deletion itself stays in the audit log
// (FR-009).
func DeleteCard(docID, cardID uint64) error {
	card, err := cardByID(docID, cardID)
	if err != nil {
		return err
	}
	if err := model.DB.Delete(&card).Error; err != nil {
		return err
	}
	summary := fmt.Sprintf("L%d 删除卡片（%d）", card.StartLine, card.ID)
	if err := writeLog(docID, logActionCardDelete, &card.ID, summary, map[string]any{
		"before": cardSummaryOf(card),
	}); err != nil {
		return err
	}
	return nil
}

// SetCardState re-adjudicates a card between pending/accepted/rejected
// (FR-008). Rejection may carry an optional reason which is kept verbatim
// when the card moves to another state.
func SetCardState(docID, cardID uint64, req *entityproofread.CardStateReq) (*model.ProofreadCard, error) {
	if req.Status != entityproofread.StatusPending &&
		req.Status != entityproofread.StatusAccepted &&
		req.Status != entityproofread.StatusRejected {
		return nil, fmt.Errorf("%w: 无效的状态（pending/accepted/rejected）", ErrBadRequest)
	}
	card, err := cardByID(docID, cardID)
	if err != nil {
		return nil, err
	}
	before := card.Status
	card.Status = req.Status
	if req.Status == entityproofread.StatusRejected {
		card.RejectReason = strings.TrimSpace(req.RejectReason)
	}
	card.UpdatedAt = time.Now()
	if err := model.DB.Save(&card).Error; err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("L%d 状态改判：%s → %s", card.StartLine, statusLabelCN(before), statusLabelCN(card.Status))
	if err := writeLog(docID, logActionCardState, &card.ID, summary, map[string]any{
		"before": before, "after": card.Status,
	}); err != nil {
		return nil, err
	}
	return card, nil
}

// cardByID loads a card scoped to its document.
func cardByID(docID, cardID uint64) (*model.ProofreadCard, error) {
	var card model.ProofreadCard
	if err := model.DB.Where("id = ? AND doc_id = ?", cardID, docID).First(&card).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 卡片 %d 不存在", ErrNotFound, cardID)
		}
		return nil, err
	}
	return &card, nil
}

// validOpType reports whether op is one of fix/replace/delete/insert.
func validOpType(op string) bool {
	return op == entityproofread.OpTypeFix || op == entityproofread.OpTypeReplace ||
		op == entityproofread.OpTypeDelete || op == entityproofread.OpTypeInsert
}

// resolveAnchor validates the requested interval against the draft line stream
// and extracts the anchored original text (FR-013: server-side authority).
func resolveAnchor(lines []string, sl, so, el, eo int, op string) (cardAnchor, string, error) {
	a := cardAnchor{StartLine: sl, StartOff: so, EndLine: el, EndOff: eo}
	if sl < 1 || el < sl || el > len(lines) {
		return a, "", fmt.Errorf("%w: 行号越界（底稿共 %d 行）", ErrBadRequest, len(lines))
	}
	first := []rune(lines[sl-1])
	last := []rune(lines[el-1])
	if so < 0 || so > len(first) {
		return a, "", fmt.Errorf("%w: L%d 起始偏移越界", ErrBadRequest, sl)
	}
	if eo < 0 || eo > len(last) {
		return a, "", fmt.Errorf("%w: L%d 结束偏移越界", ErrBadRequest, el)
	}
	if sl == el && eo < so {
		return a, "", fmt.Errorf("%w: 结束偏移先于起始偏移", ErrBadRequest)
	}
	orig, err := extractOrig(lines, a)
	if err != nil {
		return a, "", err
	}
	if op == entityproofread.OpTypeInsert {
		if sl != el || so != eo {
			return a, "", fmt.Errorf("%w: 增补操作需要空点（光标位置，无选中）", ErrBadRequest)
		}
		if orig != "" {
			return a, "", fmt.Errorf("%w: 增补操作不需要选中文本", ErrBadRequest)
		}
	}
	if op != entityproofread.OpTypeInsert && orig == "" {
		return a, "", fmt.Errorf("%w: %s操作需先选中文本", ErrBadRequest, opLabelCN(op))
	}
	return a, orig, nil
}

// extractOrig slices the anchored original text across the line stream.
func extractOrig(lines []string, a cardAnchor) (string, error) {
	if a.StartLine == a.EndLine {
		s, ok := runeSlice(lines[a.StartLine-1], a.StartOff, a.EndOff)
		if !ok {
			return "", fmt.Errorf("%w: 区间切片失败", ErrBadRequest)
		}
		return s, nil
	}
	var b strings.Builder
	for ln := a.StartLine; ln <= a.EndLine; ln++ {
		text := lines[ln-1]
		if ln == a.StartLine {
			r := []rune(text)
			if a.StartOff > len(r) {
				return "", fmt.Errorf("%w: L%d 起始偏移越界", ErrBadRequest, ln)
			}
			text = string(r[a.StartOff:])
		}
		if ln == a.EndLine {
			r := []rune(text)
			if a.EndOff > len(r) {
				return "", fmt.Errorf("%w: L%d 结束偏移越界", ErrBadRequest, ln)
			}
			text = string(r[:a.EndOff])
		}
		b.WriteString(text)
		if ln < a.EndLine {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// anchorsOverlap reports interval intersection (FR-006 semantics): ranges
// intersect when non-empty portions overlap; empty insert points conflict
// with an identical point or with a range strictly containing them.
func anchorsOverlap(a, b cardAnchor) bool {
	if a.StartLine == a.EndLine && a.StartOff == a.EndOff { // a is a point
		if b.StartLine == b.EndLine && b.StartOff == b.EndOff {
			return a.StartLine == b.StartLine && a.StartOff == b.StartOff
		}
		return pointInRange(a.StartLine, a.StartOff, b)
	}
	if b.StartLine == b.EndLine && b.StartOff == b.EndOff {
		return pointInRange(b.StartLine, b.StartOff, a)
	}
	return rangeLess(a, b) == 0
}

// rangeLess compares two non-empty ranges: -1 a<b, 1 a>b, 0 overlap.
func rangeLess(a, b cardAnchor) int {
	// by start
	if c := cmpPos(a.StartLine, a.StartOff, b.StartLine, b.StartOff); c != 0 {
		if c < 0 {
			if cmpPos(a.EndLine, a.EndOff, b.StartLine, b.StartOff) > 0 {
				return 0
			}
			return -1
		}
		if cmpPos(b.EndLine, b.EndOff, a.StartLine, a.StartOff) > 0 {
			return 0
		}
		return 1
	}
	return 0
}

// pointInRange reports whether the empty point (l,o) lies strictly inside
// range r (exclusive at both boundaries: inserts at the exact start/end of an
// existing range are adjacent, hence allowed — FR-006).
func pointInRange(l, o int, r cardAnchor) bool {
	if cmpPos(l, o, r.StartLine, r.StartOff) <= 0 {
		return false
	}
	return cmpPos(l, o, r.EndLine, r.EndOff) < 0
}

func cmpPos(l1, o1, l2, o2 int) int {
	if l1 != l2 {
		if l1 < l2 {
			return -1
		}
		return 1
	}
	if o1 != o2 {
		if o1 < o2 {
			return -1
		}
		return 1
	}
	return 0
}

// anchorOf converts a persisted card back to its anchor interval.
func anchorOf(c *model.ProofreadCard) cardAnchor {
	return cardAnchor{StartLine: c.StartLine, StartOff: c.StartOff, EndLine: c.EndLine, EndOff: c.EndOff}
}

// opLabelCN / statusLabelCN produce display labels for audit summaries.
func opLabelCN(op string) string {
	switch op {
	case entityproofread.OpTypeFix:
		return "改正"
	case entityproofread.OpTypeReplace:
		return "替换"
	case entityproofread.OpTypeDelete:
		return "删除"
	case entityproofread.OpTypeInsert:
		return "增补"
	}
	return op
}

func statusLabelCN(s string) string {
	switch s {
	case entityproofread.StatusPending:
		return "待确认"
	case entityproofread.StatusAccepted:
		return "已接受"
	case entityproofread.StatusRejected:
		return "已驳回"
	}
	return s
}

// briefDiff renders "「原文」→「拟改」" for log summaries (truncated).
func briefDiff(c *model.ProofreadCard) string {
	const max = 20
	o, r := c.OrigText, c.Replacement
	if utf8.RuneCountInString(o) > max {
		o = string([]rune(o)[:max]) + "…"
	}
	if utf8.RuneCountInString(r) > max {
		r = string([]rune(r)[:max]) + "…"
	}
	switch c.OpType {
	case entityproofread.OpTypeDelete:
		return "（删「" + o + "」）"
	case entityproofread.OpTypeInsert:
		return "（插入「" + r + "」）"
	}
	return "（「" + o + "」→「" + r + "」）"
}

// cardSummaryOf snapshots the mutable fields of a card for log detail.
func cardSummaryOf(c *model.ProofreadCard) map[string]any {
	return map[string]any{
		"op_type": c.OpType, "status": c.Status, "reason": c.Reason,
	}
}
