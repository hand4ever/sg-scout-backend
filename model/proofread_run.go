package model

import "time"

// ProofreadAutoRun is one auto-check execution record (feature 005;
// data-model.md §3). Status: running | done | partial_failed | failed.
// Engines holds the JSON array snapshot of enabled engines at start plus the
// per-engine result (name/type/config_summary/status/cards/dropped/cost_ms/error).
// Read-only after terminal state; deleted with its document (cascade, §4).
// Schema: proofread_run (user-created via schema/008-auto-proofread.sql).
type ProofreadAutoRun struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DocID      uint64     `gorm:"not null;index:idx_run_doc_time,priority:1" json:"doc_id"`
	Status     string     `gorm:"size:16;not null" json:"status"`
	Engines    string     `gorm:"type:json;not null" json:"engines"` // JSON text, decoded by service
	Summary    string     `gorm:"size:1024;not null;default:''" json:"summary"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

func (ProofreadAutoRun) TableName() string { return "proofread_run" }
