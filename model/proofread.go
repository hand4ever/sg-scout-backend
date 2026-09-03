package model

import "time"

// Proofread module models (data-model.md). Schema is created by the user via
// schema/007-proofread.sql (Constitution VII: no AutoMigrate). All columns are
// plain snake_case so no explicit column tags are required.

// ProofreadDocument is one proofreading session: a draft snapshot plus its
// source binding (crawler page | pasted text | parent revision document).
type ProofreadDocument struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SourceType   string    `gorm:"size:16;not null" json:"source_type"`       // page | text | revision
	TaskID       *uint64   `json:"task_id"`                                   // page: owning crawler task snapshot
	PageID       *uint64   `gorm:"uniqueIndex:uk_doc_page" json:"page_id"`    // page: bound crawler_page.id (1:1)
	DraftVersion *int      `json:"draft_version"`                             // page: bound page_version.version
	ParentDocID  *uint64   `gorm:"index:idx_doc_parent" json:"parent_doc_id"` // revision: parent doc
	Title        string    `gorm:"size:1024;not null;default:''" json:"title"`
	SourceURL    string    `gorm:"size:2048;not null;default:''" json:"source_url"`
	DraftText    string    `gorm:"type:longtext;not null" json:"-"` // plain line-stream snapshot
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ProofreadDocument) TableName() string { return "proofread_document" }

// ProofreadCard is one proofreading item anchored into the draft line stream.
type ProofreadCard struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	DocID         uint64    `gorm:"not null;index:idx_card_doc_order,priority:1;index:idx_card_status,priority:1" json:"doc_id"`
	OpType        string    `gorm:"size:8;not null" json:"op_type"` // fix | replace | delete | insert
	StartLine     int       `gorm:"not null;index:idx_card_doc_order,priority:2" json:"start_line"`
	StartOff      int       `gorm:"not null;index:idx_card_doc_order,priority:3" json:"start_off"`
	EndLine       int       `gorm:"not null" json:"end_line"`
	EndOff        int       `gorm:"not null" json:"end_off"`
	OrigText      string    `gorm:"type:mediumtext;not null" json:"orig_text"`
	Replacement   string    `gorm:"type:mediumtext;not null" json:"replacement"`
	Reason        string    `gorm:"size:2000;not null;default:''" json:"reason"`
	Status        string    `gorm:"size:16;not null;default:pending;index:idx_card_status,priority:2" json:"status"`
	RejectReason  string    `gorm:"size:2000;not null;default:''" json:"reject_reason"`
	AnchorVersion *int      `json:"anchor_version"` // doc.draft_version at creation
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (ProofreadCard) TableName() string { return "proofread_card" }

// ProofreadLog is an append-only audit record of proofreading operations.
type ProofreadLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	DocID     uint64    `gorm:"not null;index:idx_log_doc_time,priority:1" json:"doc_id"`
	Action    string    `gorm:"size:24;not null" json:"action"`
	CardID    *uint64   `json:"card_id"`
	Summary   string    `gorm:"size:1024;not null" json:"summary"`
	Detail    string    `gorm:"type:json" json:"detail"`
	CreatedAt time.Time `gorm:"index:idx_log_doc_time,priority:2" json:"created_at"`
}

func (ProofreadLog) TableName() string { return "proofread_log" }
