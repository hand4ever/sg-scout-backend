package model

import "time"

// ProofreadEngine is one configurable auto-proofreading engine instance
// (feature 005; data-model.md §2). engine_type ∈ lexicon|llm|httpapi
// (static registry in service/proofread/engines.go). Config holds non-secret
// type-specific settings as JSON text; secrets (provider base_url/api_key)
// stay in config.toml only (research D3). Enabled defaults to off (FR-007).
// Schema: proofread_engine (user-created via schema/008-auto-proofread.sql).
type ProofreadEngine struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	EngineType string    `gorm:"column:engine_type;size:16;not null" json:"engine_type"`
	Name       string    `gorm:"size:128;not null" json:"name"`
	Enabled    bool      `gorm:"type:tinyint(1);not null;default:false;index:idx_engine_enabled" json:"enabled"`
	Config     string    `gorm:"type:json;not null" json:"config"` // JSON text, decoded by service
	Note       string    `gorm:"size:512;not null;default:''" json:"note"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ProofreadEngine) TableName() string { return "proofread_engine" }
