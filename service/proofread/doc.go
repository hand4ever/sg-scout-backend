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
	crawlersvc "sg.scout/service/crawler"
)

// DocView is the document payload (contracts/api.md §3), draft text excluded.
type DocView struct {
	ID            uint64    `json:"id"`
	SourceType    string    `json:"source_type"`
	TaskID        *uint64   `json:"task_id"`
	PageID        *uint64   `json:"page_id"`
	DraftVersion  *int      `json:"draft_version"`
	ParentDocID   *uint64   `json:"parent_doc_id"`
	Title         string    `json:"title"`
	SourceURL     string    `json:"source_url"`
	LatestVersion *int      `json:"latest_version"` // page only, detail-time live read
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CardStats aggregates card counts per document for list views.
type CardStats struct {
	Pending  int64 `json:"pending"`
	Accepted int64 `json:"accepted"`
	Rejected int64 `json:"rejected"`
}

// DocListItem is one row of GET /proofreads (contracts §1).
type DocListItem struct {
	DocView
	Preview   string    `json:"preview"`
	CardStats CardStats `json:"card_stats"`
}

// DocDetailView is GET /proofreads/{id} payload (contracts §3).
type DocDetailView struct {
	Doc         *DocView              `json:"doc"`
	Content     string                `json:"content"`
	Cards       []model.ProofreadCard `json:"cards"`
	ParentChain []ParentNode          `json:"parent_chain"`
}

// ParentNode is one hop of the revision derivation chain (FR-021).
type ParentNode struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

// UpgradeResultView is the upgrade response (contracts §5).
type UpgradeResultView struct {
	Doc        *DocView `json:"doc"`
	ResetCount int64    `json:"reset_count"`
}

func docViewOf(d *model.ProofreadDocument) *DocView {
	return &DocView{
		ID: d.ID, SourceType: d.SourceType, TaskID: d.TaskID, PageID: d.PageID,
		DraftVersion: d.DraftVersion, ParentDocID: d.ParentDocID,
		Title: d.Title, SourceURL: d.SourceURL,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

// CreateDoc creates a page-bound (1:1) or pasted-text proofread document
// (FR-001). Page drafts snapshot the bound version's body converted to the
// plain line stream (research D1/D8).
func CreateDoc(req *entityproofread.DocCreateReq) (*DocView, error) {
	switch req.SourceType {
	case entityproofread.SourcePage:
		return createPageDoc(req)
	case entityproofread.SourceText:
		return createTextDoc(req)
	default:
		return nil, fmt.Errorf("%w: source_type 仅支持 page / text", ErrBadRequest)
	}
}

func createPageDoc(req *entityproofread.DocCreateReq) (*DocView, error) {
	if req.PageID == nil {
		return nil, fmt.Errorf("%w: source_type=page 需要 page_id", ErrBadRequest)
	}
	var page model.CrawlerPage
	if err := model.DB.First(&page, *req.PageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 页面 %d 不存在", ErrNotFound, *req.PageID)
		}
		return nil, err
	}
	if page.LatestVersion < 1 {
		return nil, fmt.Errorf("%w: 该页面尚无存档正文，无法校对", ErrBadRequest)
	}
	var exist model.ProofreadDocument
	if err := model.DB.Where("page_id = ?", *req.PageID).First(&exist).Error; err == nil {
		return nil, fmt.Errorf("%w: 该页面已有校对文档（doc %d）", ErrConflict, exist.ID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	v, err := crawlersvc.GetPageVersionContent(*req.PageID, page.LatestVersion)
	if err != nil {
		return nil, err
	}
	draft := strings.Join(MDToPlain(v.Content), "\n")
	draftVersion := page.LatestVersion
	doc := model.ProofreadDocument{
		SourceType:   entityproofread.SourcePage,
		TaskID:       &page.TaskID,
		PageID:       req.PageID,
		DraftVersion: &draftVersion,
		Title:        page.Title,
		SourceURL:    page.URL,
		DraftText:    draft,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := model.DB.Create(&doc).Error; err != nil {
		return nil, err
	}
	return docViewOf(&doc), nil
}

func createTextDoc(req *entityproofread.DocCreateReq) (*DocView, error) {
	if len(req.Content) == 0 {
		return nil, fmt.Errorf("%w: 无可校对内容（粘贴内容为空）", ErrBadRequest)
	}
	if len(req.Content) > entityproofread.MaxDraftSize {
		return nil, fmt.Errorf("%w: 内容超限：单份底稿不超过 100KB，请分段校对", ErrBadRequest)
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("%w: 无可校对内容（内容为空白）", ErrBadRequest)
	}
	doc := model.ProofreadDocument{
		SourceType: entityproofread.SourceText,
		Title:      strings.TrimSpace(req.Title),
		DraftText:  strings.TrimRight(req.Content, "\n"),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := model.DB.Create(&doc).Error; err != nil {
		return nil, err
	}
	return docViewOf(&doc), nil
}

// ListDocs lists documents, newest-updated first (limit 200). Optional
// filters: source type (all/page/text/revision) and exact page binding.
func ListDocs(source string, pageID uint64) ([]DocListItem, error) {
	q := model.DB.Model(&model.ProofreadDocument{})
	if source != "" && source != "all" {
		q = q.Where("source_type = ?", source)
	}
	if pageID > 0 {
		q = q.Where("page_id = ?", pageID)
	}
	var docs []model.ProofreadDocument
	if err := q.Order("updated_at DESC").Limit(200).Find(&docs).Error; err != nil {
		return nil, err
	}
	if docs == nil {
		docs = []model.ProofreadDocument{}
	}
	stats := loadCardStats(docs)
	items := make([]DocListItem, 0, len(docs))
	for i := range docs {
		d := &docs[i]
		items = append(items, DocListItem{
			DocView:   *docViewOf(d),
			Preview:   previewOf(d.DraftText),
			CardStats: stats[d.ID],
		})
	}
	return items, nil
}

// GetDocDetail assembles doc + draft content + ordered cards + parent chain;
// for page docs it also live-reads the crawler page's latest version so the
// UI can prompt an upgrade (FR-018). source filters the returned cards
// (feature 005 FR-008): all | manual | ignored | engine | engine:{name}.
func GetDocDetail(id uint64, source string) (*DocDetailView, error) {
	doc, err := docByID(id)
	if err != nil {
		return nil, err
	}
	dv := &DocDetailView{Doc: docViewOf(doc), Content: doc.DraftText, Cards: []model.ProofreadCard{}, ParentChain: []ParentNode{}}
	if doc.SourceType == entityproofread.SourcePage && doc.PageID != nil {
		var page model.CrawlerPage
		if err := model.DB.First(&page, *doc.PageID).Error; err == nil {
			latest := page.LatestVersion
			dv.Doc.LatestVersion = &latest
		}
	}
	q := model.DB.Where("doc_id = ?", doc.ID)
	switch {
	case source == "" || source == "all":
		// no filter — full list (004 behavior)
	case source == entityproofread.SourceManual:
		q = q.Where("source = ?", entityproofread.SourceManual)
	case source == entityproofread.StatusIgnored:
		q = q.Where("status = ?", entityproofread.StatusIgnored)
	case source == entityproofread.SourceEngine:
		q = q.Where("source = ?", entityproofread.SourceEngine)
	case strings.HasPrefix(source, "engine:"):
		q = q.Where("source = ? AND engine_name = ?", entityproofread.SourceEngine, strings.TrimPrefix(source, "engine:"))
	default:
		return nil, fmt.Errorf("%w: 无效的 source 筛选", ErrBadRequest)
	}
	if err := q.Order("start_line ASC, start_off ASC, id ASC").Find(&dv.Cards).Error; err != nil {
		return nil, err
	}
	if doc.ParentDocID != nil {
		dv.ParentChain = parentChain(doc.ParentDocID, 10)
	}
	return dv, nil
}

// UpgradeDoc rebinds a page document to the newest crawler version and resets
// all cards to pending (Q4 / FR-018). Anchor fields on cards are kept so the
// previously anchored version remains traceable.
func UpgradeDoc(id uint64) (*UpgradeResultView, error) {
	doc, err := docByID(id)
	if err != nil {
		return nil, err
	}
	if doc.SourceType != entityproofread.SourcePage || doc.PageID == nil {
		return nil, fmt.Errorf("%w: 仅页面校对文档可升级底稿", ErrBadRequest)
	}
	var page model.CrawlerPage
	if err := model.DB.First(&page, *doc.PageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 来源页面已删除，无法升级底稿", ErrConflict)
		}
		return nil, err
	}
	if doc.DraftVersion != nil && page.LatestVersion == *doc.DraftVersion {
		return nil, fmt.Errorf("%w: 底稿已是最新版本", ErrConflict)
	}
	v, err := crawlersvc.GetPageVersionContent(*doc.PageID, page.LatestVersion)
	if err != nil {
		return nil, err
	}
	draft := strings.Join(MDToPlain(v.Content), "\n")
	fromVersion := -1
	if doc.DraftVersion != nil {
		fromVersion = *doc.DraftVersion
	}
	toVersion := page.LatestVersion
	now := time.Now()
	var resetCount int64
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ProofreadDocument{}).Where("id = ?", doc.ID).Updates(map[string]any{
			"draft_text": draft, "draft_version": toVersion, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		res := tx.Model(&model.ProofreadCard{}).Where("doc_id = ?", doc.ID).Updates(map[string]any{
			"status": entityproofread.StatusPending, "updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		resetCount = res.RowsAffected
		return nil
	})
	if err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("底稿升级 v%d → v%d，重置 %d 项校对", fromVersion, toVersion, resetCount)
	if err := writeLog(doc.ID, logActionDocUpgrade, nil, summary, map[string]any{
		"from_version": fromVersion, "to_version": toVersion, "reset_count": resetCount,
	}); err != nil {
		return nil, err
	}
	upgraded, err := docByID(doc.ID)
	if err != nil {
		return nil, err
	}
	return &UpgradeResultView{Doc: docViewOf(upgraded), ResetCount: resetCount}, nil
}

// DeleteDoc deletes a document and cascades its cards and logs (FR-019).
func DeleteDoc(id uint64) error {
	doc, err := docByID(id)
	if err != nil {
		return err
	}
	return deleteDocRows(doc.ID)
}

// DeleteByPage removes every document bound to a page (cascade entry point
// used by the crawler delete handler; idempotent — no rows is a no-op).
func DeleteByPage(pageID uint64) error {
	var docs []model.ProofreadDocument
	if err := model.DB.Where("page_id = ?", pageID).Find(&docs).Error; err != nil {
		return err
	}
	for _, d := range docs {
		if err := deleteDocRows(d.ID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByTask removes page documents whose owning task is deleted
// (idempotent; text/revision documents have no task binding and are skipped).
func DeleteByTask(taskID uint64) error {
	var docs []model.ProofreadDocument
	if err := model.DB.Where("task_id = ?", taskID).Find(&docs).Error; err != nil {
		return err
	}
	for _, d := range docs {
		if err := deleteDocRows(d.ID); err != nil {
			return err
		}
	}
	return nil
}

// docByID loads one document by id (404 wrapper).
func docByID(id uint64) (*model.ProofreadDocument, error) {
	var doc model.ProofreadDocument
	if err := model.DB.First(&doc, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 校对文档 %d 不存在", ErrNotFound, id)
		}
		return nil, err
	}
	return &doc, nil
}

// deleteDocRows removes one document's run/log/card/doc rows in a transaction
// (005 extends the 004 cascade with auto runs, data-model §4).
func deleteDocRows(docID uint64) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("doc_id = ?", docID).Delete(&model.ProofreadAutoRun{}).Error; err != nil {
			return err
		}
		if err := tx.Where("doc_id = ?", docID).Delete(&model.ProofreadLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("doc_id = ?", docID).Delete(&model.ProofreadCard{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", docID).Delete(&model.ProofreadDocument{}).Error
	})
}

// loadCardStats counts card statuses for a batch of documents (one query).
func loadCardStats(docs []model.ProofreadDocument) map[uint64]CardStats {
	stats := make(map[uint64]CardStats, len(docs))
	if len(docs) == 0 {
		return stats
	}
	ids := make([]uint64, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.ID)
		stats[d.ID] = CardStats{}
	}
	type row struct {
		DocID  uint64
		Status string
		Cnt    int64
	}
	var rows []row
	if err := model.DB.Model(&model.ProofreadCard{}).
		Select("doc_id, status, COUNT(*) AS cnt").
		Where("doc_id IN ?", ids).Group("doc_id, status").Scan(&rows).Error; err != nil {
		return stats
	}
	for _, r := range rows {
		s := stats[r.DocID]
		switch r.Status {
		case entityproofread.StatusPending:
			s.Pending = r.Cnt
		case entityproofread.StatusAccepted:
			s.Accepted = r.Cnt
		case entityproofread.StatusRejected:
			s.Rejected = r.Cnt
		}
		stats[r.DocID] = s
	}
	return stats
}

// previewOf returns the first non-empty line truncated to 40 runes.
func previewOf(draft string) string {
	for _, line := range strings.Split(draft, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 40 {
			r = r[:40]
		}
		return string(r)
	}
	return ""
}

// parentChain walks parent_doc_id up to maxHops collecting {id,title} hops in
// root→child order (FR-021 traceability).
func parentChain(parent *uint64, maxHops int) []ParentNode {
	nodes := []ParentNode{}
	id := *parent
	for i := 0; i < maxHops; i++ {
		var p model.ProofreadDocument
		if err := model.DB.First(&p, id).Error; err != nil {
			break
		}
		nodes = append(nodes, ParentNode{ID: p.ID, Title: p.Title})
		if p.ParentDocID == nil {
			break
		}
		id = *p.ParentDocID
	}
	// reverse to root-first
	for l, r := 0, len(nodes)-1; l < r; l, r = l+1, r-1 {
		nodes[l], nodes[r] = nodes[r], nodes[l]
	}
	return nodes
}

// draftLines splits the stored draft back into its line stream (1-based lines
// correspond to index i-1).
func draftLines(draft string) []string {
	return strings.Split(draft, "\n")
}

// runeSlice returns the rune range [start,end) of line as a string; ok=false
// when the range is invalid for that line.
func runeSlice(line string, start, end int) (string, bool) {
	r := []rune(line)
	if start < 0 || end < start || end > len(r) {
		return "", false
	}
	return string(r[start:end]), true
}

// runeLen reports the rune length of a string.
func runeLen(s string) int { return utf8.RuneCountInString(s) }
