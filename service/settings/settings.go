// Package settings implements the runtime system-config source (feature 002
// FR-011/012): keys registered in code, DB authoritative after first seed,
// config.toml only seeds initial values and keeps secrets. Consumers read via
// GetString/GetInt (each read hits the DB = immediate effect by design).
package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"sg.scout/config"
	"sg.scout/model"
	"sg.scout/service/crawler/engine"
)

// Keys of the v1 registry (data-model.md §3) + proofreading defaults
// (feature 005, added 2026-09-04: model & effort for new llm engines).
const (
	KeyDefaultEngine        = "default_engine"
	KeySchedulerConcurrency = "scheduler_concurrency"
	KeyStorageRoot          = "storage_root"

	KeyProofreadDefaultModel = "proofread_default_model" // 新建大模型引擎的默认模型
	KeyProofreadEffort       = "proofread_effort"        // 新建大模型引擎的默认思考强度
)

// EffortOptions are the accepted effort values (deepseek V4 thinking modes).
var EffortOptions = []string{"none", "low", "high", "max"}

// ModelOptions are the suggested model ids for the datalist picker.
var ModelOptions = []string{"deepseek-v4-flash", "deepseek-v4-pro"}

// Effect marks when a setting takes effect (shown in the settings UI).
type Effect string

const (
	EffectImmediate Effect = "immediate" // applied to newly created tasks/runs
	EffectRestart   Effect = "restart"   // read once at startup (workers/storage)
)

// Def declares one registered setting.
type Def struct {
	Key     string
	Type    string // "string" | "int"
	Default any    // code default (used when neither DB nor config.toml provides)
	Effect  Effect
	Desc    string
	Options []string // non-empty → UI renders a select (string keys only)
}

var registry = []Def{
	{Key: KeyDefaultEngine, Type: "string", Default: "goquery", Effect: EffectImmediate,
		Desc: "新建任务的默认抓取引擎"},
	{Key: KeySchedulerConcurrency, Type: "int", Default: 1, Effect: EffectRestart,
		Desc: "任务执行并发数（worker 池启动时创建）"},
	{Key: KeyStorageRoot, Type: "string", Default: "./data", Effect: EffectRestart,
		Desc: "内容落盘根目录（启动时初始化）"},
	{Key: KeyProofreadDefaultModel, Type: "string", Default: "deepseek-v4-flash", Effect: EffectImmediate,
		Desc: "新建大模型校对引擎的默认模型（deepseek V4 系列）", Options: ModelOptions},
	{Key: KeyProofreadEffort, Type: "string", Default: "none", Effect: EffectImmediate,
		Desc: "新建大模型校对引擎的默认思考强度（none=关思考/快）", Options: EffortOptions},
}

// Lookup returns the registered definition of key.
func Lookup(key string) (Def, bool) {
	for _, d := range registry {
		if d.Key == key {
			return d, true
		}
	}
	return Def{}, false
}

// RegisteredKeys returns all registry keys (declaration order).
func RegisteredKeys() []string {
	out := make([]string, 0, len(registry))
	for _, d := range registry {
		out = append(out, d.Key)
	}
	return out
}

// seedValue returns the config.toml/code default used for first seeding.
// config.toml [crawler.engine] provider seeds default_engine (legacy installs
// keep their current engine); crawler defaults seed the rest (data-model §3).
func seedValue(d Def) any {
	cfg := config.Cfg.Crawler
	switch d.Key {
	case KeyDefaultEngine:
		if p := strings.TrimSpace(cfg.Engine.Provider); p != "" {
			return p
		}
		return engine.EngineGoquery
	case KeySchedulerConcurrency:
		if cfg.Concurrency > 0 {
			return cfg.Concurrency
		}
		return 1
	case KeyStorageRoot:
		if s := strings.TrimSpace(cfg.StorageRoot); s != "" {
			return s
		}
		return "./data"
	}
	return d.Default
}

func marshalVal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Seed inserts missing registry keys once at startup (never overwrites
// existing DB rows = DB stays authoritative). Not audited (no change).
func Seed() error {
	for _, d := range registry {
		var n int64
		if err := model.DB.Model(&model.SystemSetting{}).Where("skey = ?", d.Key).Count(&n).Error; err != nil {
			return fmt.Errorf("settings seed count %s: %w", d.Key, err)
		}
		if n > 0 {
			continue
		}
		now := time.Now()
		row := &model.SystemSetting{
			SKey: d.Key, SValue: marshalVal(seedValue(d)), CreatedAt: now, UpdatedAt: now,
		}
		if err := model.DB.Create(row).Error; err != nil {
			return fmt.Errorf("settings seed insert %s: %w", d.Key, err)
		}
		log.Printf("[settings] seeded %s=%s", d.Key, row.SValue)
	}
	return nil
}

// getRow loads one setting row (DB authoritative; missing row = error).
func getRow(key string) (*model.SystemSetting, error) {
	var row model.SystemSetting
	if err := model.DB.Where("skey = ?", key).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// GetString reads a string setting. ok=false when missing or type mismatch.
func GetString(key string) (string, bool) {
	def, ok := Lookup(key)
	if !ok || def.Type != "string" {
		return "", false
	}
	row, err := getRow(key)
	if err != nil {
		if row == nil && err == gorm.ErrRecordNotFound {
			// Row not seeded yet (e.g. tests) — fall back to registry default.
			s, _ := def.Default.(string)
			return s, true
		}
		return "", false
	}
	var s string
	if err := row.Value(&s); err != nil {
		return "", false
	}
	return s, true
}

// GetInt reads an int setting. ok=false when missing or type mismatch.
func GetInt(key string) (int, bool) {
	def, ok := Lookup(key)
	if !ok || def.Type != "int" {
		return 0, false
	}
	row, err := getRow(key)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			if i, ok := def.Default.(int); ok {
				return i, true
			}
		}
		return 0, false
	}
	var i int
	if err := row.Value(&i); err != nil {
		return 0, false
	}
	return i, true
}

// Item is one setting row for GET /crawler/settings.
type Item struct {
	Key         string   `json:"key"`
	Value       any      `json:"value"`
	Default     any      `json:"default_value"`
	Effect      Effect   `json:"effect"`
	Description string   `json:"description"`
	Options     []string `json:"options,omitempty"` // non-empty → UI renders a select
}

// Items returns every registered setting with its current value and metadata.
func Items() ([]Item, error) {
	out := make([]Item, 0, len(registry))
	for _, d := range registry {
		row, err := getRow(d.Key)
		if err != nil {
			return nil, fmt.Errorf("settings read %s: %w", d.Key, err)
		}
		var v any
		if d.Type == "int" {
			var i int
			if err := row.Value(&i); err != nil {
				return nil, err
			}
			v = i
		} else {
			var s string
			if err := row.Value(&s); err != nil {
				return nil, err
			}
			v = s
		}
		out = append(out, Item{Key: d.Key, Value: v, Default: d.Default, Effect: d.Effect, Description: d.Desc, Options: d.Options})
	}
	return out, nil
}

// Validate checks a value against the registered key type/constraints.
// Used by task creation (default_engine) and the settings API (US4).
func Validate(key string, v any) error {
	def, ok := Lookup(key)
	if !ok {
		return fmt.Errorf("未知配置键 %q（可用：%s）", key, strings.Join(RegisteredKeys(), ", "))
	}
	switch def.Type {
	case "string":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("配置 %s 需要字符串值", key)
		}
		if s == "" {
			return fmt.Errorf("配置 %s 不能为空", key)
		}
		if len(def.Options) > 0 && !slices.Contains(def.Options, s) {
			return fmt.Errorf("配置 %s 取值无效（可选：%s）", key, strings.Join(def.Options, " / "))
		}
		if key == KeyDefaultEngine && !engine.Lookup(s) {
			return fmt.Errorf("未知引擎 %q（可用：firecrawl / crawl4ai / goquery）", s)
		}
	case "int":
		i, ok := v.(int)
		if !ok {
			if f, ok2 := v.(float64); ok2 { // JSON decode yields float64
				i, ok = int(f), true
			}
		}
		if !ok {
			return fmt.Errorf("配置 %s 需要整数", key)
		}
		if key == KeySchedulerConcurrency && i < 1 {
			return fmt.Errorf("配置 %s 必须 ≥1", key)
		}
	}
	return nil
}

// normalize coerces a JSON-decoded value into the registered type.
func normalize(key string, v any) (any, error) {
	def, ok := Lookup(key)
	if !ok {
		return nil, fmt.Errorf("未知配置键 %q", key)
	}
	switch def.Type {
	case "string":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("配置 %s 需要字符串", key)
		}
		return s, nil
	case "int":
		switch n := v.(type) {
		case int:
			return n, nil
		case float64:
			return int(n), nil
		default:
			return nil, fmt.Errorf("配置 %s 需要整数", key)
		}
	}
	return v, nil
}

// writeLocked applies one key/value change with audit inside a transaction.
func writeLocked(tx *gorm.DB, key string, newVal any, note string) error {
	now := time.Now()
	var row model.SystemSetting
	err := tx.Where("skey = ?", key).First(&row).Error
	if err != nil {
		return err
	}
	oldRaw := row.SValue
	newRaw := marshalVal(newVal)
	if oldRaw == newRaw {
		return nil // no-op: skip audit noise
	}
	if err := tx.Model(&model.SystemSetting{}).Where("skey = ?", key).
		Updates(map[string]any{"svalue": newRaw, "note": note, "updated_at": now}).Error; err != nil {
		return err
	}
	return tx.Create(&model.SystemSettingLog{
		SKey: key, OldValue: oldRaw, NewValue: newRaw, Note: note, CreatedAt: now,
	}).Error
}

// UpdateItems validates all items first, then applies each with one audit row
// per changed key (partial update semantics, FR-013). Restart-effect keys
// return their effect label so the caller can surface a hint.
func UpdateItems(items map[string]any, note string) ([]string, error) {
	restartKeys := []string{}
	normalized := map[string]any{}
	for key, v := range items {
		if err := Validate(key, v); err != nil {
			return nil, err
		}
		nv, err := normalize(key, v)
		if err != nil {
			return nil, err
		}
		normalized[key] = nv
		if d, _ := Lookup(key); d.Effect == EffectRestart {
			restartKeys = append(restartKeys, key)
		}
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("items 不能为空")
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for key, nv := range normalized {
			if err := writeLocked(tx, key, nv, note); err != nil {
				return fmt.Errorf("更新 %s: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return restartKeys, nil
}

// Reset restores key (or all keys when key is empty) to its default value,
// auditing each change. Returns keys that take effect after restart.
func Reset(key, note string) ([]string, error) {
	targets := RegisteredKeys()
	if key != "" {
		if _, ok := Lookup(key); !ok {
			return nil, fmt.Errorf("未知配置键 %q", key)
		}
		targets = []string{key}
	}
	restartKeys := []string{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, k := range targets {
			d, _ := Lookup(k)
			if err := writeLocked(tx, k, seedValue(d), note); err != nil {
				return fmt.Errorf("重置 %s: %w", k, err)
			}
			if d.Effect == EffectRestart {
				restartKeys = append(restartKeys, k)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return restartKeys, nil
}

// History returns the audit trail of key (all keys when empty), newest first.
func History(key string, limit int) ([]model.SystemSettingLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := model.DB.Model(&model.SystemSettingLog{})
	if key != "" {
		q = q.Where("skey = ?", key)
	}
	var rows []model.SystemSettingLog
	if err := q.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
