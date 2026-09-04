package proofread

// EngineCreateReq is the POST /proofreads/engines body (contracts §3).
// Enabled defaults to false (FR-007); explicit enabled=true triggers the
// deep validation (dict file readable / provider configured).
type EngineCreateReq struct {
	EngineType string         `json:"engine_type"` // lexicon | llm | httpapi
	Name       string         `json:"name"`
	Enabled    bool           `json:"enabled"`
	Config     map[string]any `json:"config"`
	Note       string         `json:"note"`
}

// EngineUpdateReq is the PATCH /proofreads/engines/{eid} body (contracts §4).
type EngineUpdateReq struct {
	Name    *string         `json:"name"`
	Enabled *bool           `json:"enabled"`
	Config  *map[string]any `json:"config"`
	Note    *string         `json:"note"`
}

// Engine type ids (static registry; service/proofread/engines.go).
const (
	EngineTypeLexicon = "lexicon" // 词库比对
	EngineTypeLLM     = "llm"     // 大模型校对
	EngineTypeHTTPAPI = "httpapi" // 第三方校对 API（适配器预留）
)

// Auto-run statuses (data-model.md §3).
const (
	RunStatusRunning       = "running"
	RunStatusDone          = "done"
	RunStatusPartialFailed = "partial_failed"
	RunStatusFailed        = "failed"
)

// Card source values (data-model.md §1).
const (
	SourceManual = "manual" // 人工建卡（004）
	SourceEngine = "engine" // 自动引擎产出（005）
)

// MaxEngineNameLen caps the engine display name.
const MaxEngineNameLen = 128
