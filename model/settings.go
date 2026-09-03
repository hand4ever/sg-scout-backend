package model

import (
	"encoding/json"
	"time"
)

// SystemSetting is one runtime-config key/value pair (feature 002 FR-011/012:
// DB is the authoritative config source; config.toml only seeds defaults and
// keeps secrets). SValue holds the raw JSON text of the value (e.g. "goquery").
// Schema: system_settings (user-created via schema/crawler.sql / 002 migration).
type SystemSetting struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SKey      string    `gorm:"column:skey;size:64;not null;uniqueIndex:uk_settings_key" json:"skey"`
	SValue    string    `gorm:"column:svalue;type:json;not null" json:"svalue"`
	Note      string    `gorm:"size:512" json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// Value decodes the stored JSON text into out (e.g. &"", &0).
func (s *SystemSetting) Value(out any) error { return json.Unmarshal([]byte(s.SValue), out) }

// SystemSettingLog records one settings change (audit + rollback support).
type SystemSettingLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SKey      string    `gorm:"column:skey;size:64;not null;index:idx_settings_log_key_time,priority:1" json:"skey"`
	OldValue  string    `gorm:"column:old_value;type:json" json:"old_value"`
	NewValue  string    `gorm:"column:new_value;type:json;not null" json:"new_value"`
	Note      string    `gorm:"size:512" json:"note"`
	CreatedAt time.Time `gorm:"index:idx_settings_log_key_time,priority:2" json:"created_at"`
}

func (SystemSettingLog) TableName() string { return "system_settings_log" }
