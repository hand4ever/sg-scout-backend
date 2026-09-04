// Package proofread implements the proofreading module (feature 004/005).
// engines.go: engine type registry (static) + engine instance CRUD (DB rows,
// research D1/D2). Secrets live in config.toml [proofread.providers.*]; the DB
// row only references a provider by name (research D3).
package proofread

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"sg.scout/config"
	entityproofread "sg.scout/entity/proofread"
	"sg.scout/model"
	settingsvc "sg.scout/service/settings"
)

// --- Static engine type registry (research D1) ---

// ConfigField declares one config key of an engine type (drives the settings
// UI form, contracts §1).
type ConfigField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"` // string | int
	Required bool   `json:"required"`
	Hint     string `json:"hint"`
}

// TypeInfo is the static description of one engine type (contracts §1).
type TypeInfo struct {
	Type          string        `json:"type"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Fields        []ConfigField `json:"fields"`
	NeedsProvider bool          `json:"needs_provider"`
	Providers     []string      `json:"providers,omitempty"` // configured provider names (llm/httpapi)
}

const defaultMaxChars = 30000 // llm single-call input cap (research D13)

var engineTypes = []TypeInfo{
	{
		Type: entityproofread.EngineTypeLexicon, Name: "词库比对",
		Description: "按错词→正词词库精确比对全文，毫秒级、零外部依赖",
		Fields: []ConfigField{
			{Key: "dict_path", Label: "词库文件路径", Type: "string", Required: true,
				Hint: "文本文件，每行 错词→正词（# 注释）；运行每次重读"},
		},
		NeedsProvider: false,
	},
	{
		Type: entityproofread.EngineTypeLLM, Name: "大模型校对",
		Description: "经统一大模型服务接口（OpenAI 兼容）逐段校对，多个模型实例可并存（模型 A/B）",
		Fields: []ConfigField{
			{Key: "provider", Label: "校对服务", Type: "string", Required: true,
				Hint: "config.toml [proofread.providers.*] 中已配置的服务名"},
			{Key: "model", Label: "模型标识", Type: "string", Required: true, Hint: "如 deepseek-v4-flash"},
			{Key: "effort", Label: "思考强度", Type: "string", Required: false,
				Hint: "默认 none=关思考(快、稳定)；可选 low/high/max 开启深度思考（更慢）；thinking 模式下 temperature 不生效（V4 调用方式）"},
			{Key: "max_chars", Label: "单次输入上限(字符)", Type: "int", Required: false,
				Hint: "默认 30000；超长底稿该引擎失败并提示分段"},
		},
		NeedsProvider: true,
	},
	{
		Type: entityproofread.EngineTypeHTTPAPI, Name: "第三方校对 API",
		Description: "经 HTTP 适配器调用第三方校对服务（响应须符合候选契约，见 contracts 附录 A）",
		Fields: []ConfigField{
			{Key: "provider", Label: "校对服务", Type: "string", Required: true,
				Hint: "config.toml [proofread.providers.*] 中已配置的服务名"},
		},
		NeedsProvider: true,
	},
}

// ListTypes returns the static engine type registry with live provider names
// injected for needs_provider types (contracts §1).
func ListTypes() []TypeInfo {
	out := make([]TypeInfo, len(engineTypes))
	copy(out, engineTypes)
	for i := range out {
		if out[i].NeedsProvider {
			out[i].Providers = ProviderNames()
		}
	}
	return out
}

// ProviderNames lists provider keys declared in config.toml [proofread.providers].
func ProviderNames() []string {
	var names []string
	for name := range config.Cfg.Proofread.Providers {
		names = append(names, name)
	}
	if names == nil {
		names = []string{}
	}
	return names
}

// providerConfigured reports whether name exists in config.toml.
func providerConfigured(name string) bool {
	_, ok := config.Cfg.Proofread.Providers[name]
	return ok
}

// lookupType returns the static type info by id.
func lookupType(t string) (*TypeInfo, bool) {
	for i := range engineTypes {
		if engineTypes[i].Type == t {
			return &engineTypes[i], true
		}
	}
	return nil, false
}

// --- Instance config handling ---

// decodeConfigJSON parses the stored JSON text column into a map.
func decodeConfigJSON(raw string) (map[string]any, error) {
	m := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("引擎配置 JSON 解析失败：%v", err)
	}
	return m, nil
}

// encodeConfigJSON serializes a config map for the JSON column.
func encodeConfigJSON(m map[string]any) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// strField returns a required string field from a config map.
func strField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return strings.TrimSpace(s), ok && strings.TrimSpace(s) != ""
}

// validateConfig checks a config map against its type's field schema
// (missing required fields / wrong types → error).
func validateConfig(engineType string, m map[string]any) error {
	ti, ok := lookupType(engineType)
	if !ok {
		return fmt.Errorf("%w: 未知引擎类型 %q", ErrBadRequest, engineType)
	}
	for _, f := range ti.Fields {
		v, present := m[f.Key]
		if f.Type == "int" {
			if !present {
				continue // ints optional (defaults applied at runtime)
			}
			switch n := v.(type) {
			case float64:
				if n <= 0 {
					return fmt.Errorf("%w: 配置 %s 需要正整数", ErrBadRequest, f.Label)
				}
			case int:
				if n <= 0 {
					return fmt.Errorf("%w: 配置 %s 需要正整数", ErrBadRequest, f.Label)
				}
			default:
				return fmt.Errorf("%w: 配置 %s 需要整数", ErrBadRequest, f.Label)
			}
			continue
		}
		if !f.Required {
			continue
		}
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return fmt.Errorf("%w: 缺少必填配置 %s", ErrBadRequest, f.Label)
		}
	}
	return nil
}

// validateEnabled performs the deep validation required before an instance can
// run (FR-007): lexicon dict file must be readable with ≥1 valid entry; llm /
// httpapi provider must exist in config.toml (research D3).
func validateEnabled(engineType string, m map[string]any) error {
	switch engineType {
	case entityproofread.EngineTypeLexicon:
		path, _ := strField(m, "dict_path")
		if path == "" {
			return fmt.Errorf("%w: 词库引擎缺少词库文件路径（dict_path）", ErrBadRequest)
		}
		if _, err := LoadLexiconFile(path); err != nil {
			return fmt.Errorf("%w: %v", ErrBadRequest, err)
		}
	case entityproofread.EngineTypeLLM, entityproofread.EngineTypeHTTPAPI:
		provider, _ := strField(m, "provider")
		if provider == "" {
			return fmt.Errorf("%w: %s引擎缺少校对服务（provider）", ErrBadRequest, engineType)
		}
		if !providerConfigured(provider) {
			return fmt.Errorf("%w: 校对服务 %q 未在 config.toml [proofread.providers] 配置（密钥只存配置文件，不落库）", ErrBadRequest, provider)
		}
		if engineType == entityproofread.EngineTypeLLM {
			if _, ok := strField(m, "model"); !ok {
				return fmt.Errorf("%w: 大模型引擎缺少模型标识（model）", ErrBadRequest)
			}
			// Effort must be one of the registered thinking levels (global
			// config proofread_effort enum mirrors these values).
			if e, ok := strField(m, "effort"); ok {
				valid := false
				for _, opt := range settingsvc.EffortOptions {
					if e == opt {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("%w: 思考强度 %q 无效（可选：none/low/high/max）", ErrBadRequest, e)
				}
			}
		}
	}
	return nil
}

// --- Instance CRUD (contracts §2-5) ---

// EngineView is one instance row for the API.
type EngineView struct {
	ID         uint64         `json:"id"`
	EngineType string         `json:"engine_type"`
	Name       string         `json:"name"`
	Enabled    bool           `json:"enabled"`
	Config     map[string]any `json:"config"`
	Note       string         `json:"note"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func engineViewOf(e *model.ProofreadEngine) (*EngineView, error) {
	m, err := decodeConfigJSON(e.Config)
	if err != nil {
		return nil, err
	}
	return &EngineView{
		ID: e.ID, EngineType: e.EngineType, Name: e.Name, Enabled: e.Enabled,
		Config: m, Note: e.Note, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}, nil
}

// ListEngines returns all engine instances ordered by id.
func ListEngines() ([]EngineView, error) {
	var rows []model.ProofreadEngine
	if err := model.DB.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EngineView, 0, len(rows))
	for i := range rows {
		v, err := engineViewOf(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// CreateEngine persists a new engine instance (contracts §3). Deep validation
// runs only when enabled=true (FR-007 default off).
func CreateEngine(req *entityproofread.EngineCreateReq) (*EngineView, error) {
	if _, ok := lookupType(req.EngineType); !ok {
		return nil, fmt.Errorf("%w: 未知引擎类型 %q（可用：lexicon / llm / httpapi）", ErrBadRequest, req.EngineType)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || runeLen(name) > entityproofread.MaxEngineNameLen {
		return nil, fmt.Errorf("%w: 引擎名称必填且不超过 %d 字符", ErrBadRequest, entityproofread.MaxEngineNameLen)
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	// LLM defaults from global settings (feature 005: 全局配置驱动新建引擎默认).
	if req.EngineType == entityproofread.EngineTypeLLM {
		if _, ok := req.Config["model"]; !ok {
			if m, ok := settingsvc.GetString(settingsvc.KeyProofreadDefaultModel); ok {
				req.Config["model"] = m
			}
		}
		if _, ok := req.Config["effort"]; !ok {
			if e, ok := settingsvc.GetString(settingsvc.KeyProofreadEffort); ok {
				req.Config["effort"] = e
			}
		}
	}
	if err := validateConfig(req.EngineType, req.Config); err != nil {
		return nil, err
	}
	if req.Enabled {
		if err := validateEnabled(req.EngineType, req.Config); err != nil {
			return nil, err
		}
	}
	cfgRaw, err := encodeConfigJSON(req.Config)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	e := model.ProofreadEngine{
		EngineType: req.EngineType, Name: name, Enabled: req.Enabled,
		Config: cfgRaw, Note: strings.TrimSpace(req.Note), CreatedAt: now, UpdatedAt: now,
	}
	if err := model.DB.Create(&e).Error; err != nil {
		return nil, err
	}
	return engineViewOf(&e)
}

// UpdateEngine applies a partial update (contracts §4). Enabling (0→1) runs
// the deep validation; config changes affect future runs only.
func UpdateEngine(id uint64, req *entityproofread.EngineUpdateReq) (*EngineView, error) {
	var e model.ProofreadEngine
	if err := model.DB.First(&e, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 引擎 %d 不存在", ErrNotFound, id)
		}
		return nil, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || runeLen(name) > entityproofread.MaxEngineNameLen {
			return nil, fmt.Errorf("%w: 引擎名称必填且不超过 %d 字符", ErrBadRequest, entityproofread.MaxEngineNameLen)
		}
		e.Name = name
	}
	configChanged := false
	if req.Config != nil {
		if err := validateConfig(e.EngineType, *req.Config); err != nil {
			return nil, err
		}
		raw, err := encodeConfigJSON(*req.Config)
		if err != nil {
			return nil, err
		}
		e.Config = raw
		configChanged = true
	}
	if req.Note != nil {
		e.Note = strings.TrimSpace(*req.Note)
	}
	if req.Enabled != nil && *req.Enabled && !e.Enabled {
		// enabling (or already enabled + config changed): validate runnability
		m, err := decodeConfigJSON(e.Config)
		if err != nil {
			return nil, err
		}
		if err := validateEnabled(e.EngineType, m); err != nil {
			return nil, err
		}
		e.Enabled = true
	} else if req.Enabled != nil {
		e.Enabled = *req.Enabled
	} else if configChanged && e.Enabled {
		m, err := decodeConfigJSON(e.Config)
		if err != nil {
			return nil, err
		}
		if err := validateEnabled(e.EngineType, m); err != nil {
			return nil, err
		}
	}
	e.UpdatedAt = time.Now()
	if err := model.DB.Save(&e).Error; err != nil {
		return nil, err
	}
	return engineViewOf(&e)
}

// DeleteEngine removes an engine instance row. Already-produced cards keep
// their engine_name/run_id snapshot (research D12) — no cascade.
func DeleteEngine(id uint64) error {
	var e model.ProofreadEngine
	if err := model.DB.First(&e, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: 引擎 %d 不存在", ErrNotFound, id)
		}
		return err
	}
	return model.DB.Delete(&e).Error
}

// --- Enabled list snapshot (research D14 / contracts §6) ---

// EngineSnapshot is one engine row as captured by an auto run (non-secret
// config summary only).
type EngineSnapshot struct {
	EngineID      uint64 `json:"engine_id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	ConfigSummary string `json:"config_summary"`
}

// ConfigSummaryOf renders a one-line non-secret summary for snapshots/UI.
func ConfigSummaryOf(engineType string, m map[string]any) string {
	switch engineType {
	case entityproofread.EngineTypeLexicon:
		if p, ok := strField(m, "dict_path"); ok {
			return "dict_path=" + p
		}
	case entityproofread.EngineTypeLLM:
		prov, _ := strField(m, "provider")
		model, _ := strField(m, "model")
		return fmt.Sprintf("provider=%s, model=%s", prov, model)
	case entityproofread.EngineTypeHTTPAPI:
		if p, ok := strField(m, "provider"); ok {
			return "provider=" + p
		}
	}
	return ""
}

// EnabledSnapshots returns snapshots of all enabled engine instances
// (id order) for a new auto run (FR-002: 开始校对 = all enabled engines).
func EnabledSnapshots() ([]EngineSnapshot, error) {
	var rows []model.ProofreadEngine
	if err := model.DB.Where("enabled = ?", true).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EngineSnapshot, 0, len(rows))
	for i := range rows {
		m, err := decodeConfigJSON(rows[i].Config)
		if err != nil {
			return nil, err
		}
		out = append(out, EngineSnapshot{
			EngineID: rows[i].ID, Name: rows[i].Name, Type: rows[i].EngineType,
			ConfigSummary: ConfigSummaryOf(rows[i].EngineType, m),
		})
	}
	return out, nil
}

// EngineRunConfig decodes one snapshot back to its config map for execution
// (looked up live by engine id at run time so edits apply on next run).
func EngineConfigByID(id uint64) (map[string]any, string, error) {
	var e model.ProofreadEngine
	if err := model.DB.First(&e, id).Error; err != nil {
		return nil, "", err
	}
	m, err := decodeConfigJSON(e.Config)
	return m, e.Name, err
}
