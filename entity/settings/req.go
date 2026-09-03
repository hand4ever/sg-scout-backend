// Package settingsentity holds request payloads for the system settings API
// (feature 002 US4, contracts/api.md).
package settings

// UpdateSettingsReq is the PUT /crawler/settings body (partial update).
type UpdateSettingsReq struct {
	Items []SettingUpdate `json:"items"`
	Note  string          `json:"note"`
}

// SettingUpdate is one key/value pair to update.
type SettingUpdate struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// ResetSettingsReq is the POST /crawler/settings/reset body.
// Empty Key resets every registered key to its default.
type ResetSettingsReq struct {
	Key  string `json:"key"`
	Note string `json:"note"`
}
