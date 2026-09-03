package proofread

import (
	"encoding/json"
	"time"

	"sg.scout/model"
)

// Proofread log actions (append-only audit; FR-012/013).
const (
	logActionCardCreate = "card_create"
	logActionCardUpdate = "card_update"
	logActionCardDelete = "card_delete"
	logActionCardState  = "card_state"
	logActionDocUpgrade = "doc_upgrade"
)

// writeLog appends one audit entry. detail, when non-nil, is stored as JSON
// (before/after field summary only — no full-text payloads).
func writeLog(docID uint64, action string, cardID *uint64, summary string, detail any) error {
	var detailJSON string
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		detailJSON = string(b)
	}
	l := model.ProofreadLog{
		DocID:     docID,
		Action:    action,
		CardID:    cardID,
		Summary:   summary,
		Detail:    detailJSON,
		CreatedAt: time.Now(),
	}
	return model.DB.Create(&l).Error
}

// ListLogs returns a document's audit trail, newest first (limit 200).
// Read-only: no update/delete path exists for logs (FR-013). A missing
// document is a 404 (contracts §10).
func ListLogs(docID uint64) ([]model.ProofreadLog, error) {
	if _, err := docByID(docID); err != nil {
		return nil, err
	}
	var list []model.ProofreadLog
	if err := model.DB.Where("doc_id = ?", docID).
		Order("created_at DESC, id DESC").Limit(200).Find(&list).Error; err != nil {
		return nil, err
	}
	if list == nil {
		list = []model.ProofreadLog{}
	}
	return list, nil
}
