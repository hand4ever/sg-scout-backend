// Package proofread holds request payloads for the proofread module
// (contracts/api.md, feature 004).
package proofread

// DocCreateReq is the POST /proofreads body. source_type selects the entry:
// "page" (bind a crawled page 1:1) or "text" (paste arbitrary text).
type DocCreateReq struct {
	SourceType string  `json:"source_type"`
	PageID     *uint64 `json:"page_id"` // required when source_type=page
	Title      string  `json:"title"`   // optional; text entries may name the draft
	Content    string  `json:"content"` // required when source_type=text
}

// CardCreateReq is the POST /proofreads/{id}/cards body. The anchored original
// text is NOT client-supplied: the service extracts it from the draft (FR-013).
type CardCreateReq struct {
	OpType      string `json:"op_type"` // fix | replace | delete | insert
	StartLine   int    `json:"start_line"`
	StartOff    int    `json:"start_off"`
	EndLine     int    `json:"end_line"`
	EndOff      int    `json:"end_off"`
	Replacement string `json:"replacement"`
	Reason      string `json:"reason"`
}

// CardUpdateReq is the PATCH body (partial update). Editing fields never
// changes the card status (spec Q6 / FR-008).
type CardUpdateReq struct {
	OpType      *string `json:"op_type"`
	Replacement *string `json:"replacement"`
	Reason      *string `json:"reason"`
}

// CardStateReq is the POST .../state body for status re-adjudication.
type CardStateReq struct {
	Status       string `json:"status"` // pending | accepted | rejected
	RejectReason string `json:"reject_reason"`
}

// Type/status/source constants (contracts/api.md + data-model.md).
const (
	OpTypeFix     = "fix"     // 改正：局部修正错字/词/标点
	OpTypeReplace = "replace" // 替换：整段/整句整体重写
	OpTypeDelete  = "delete"  // 删除：删去选中内容
	OpTypeInsert  = "insert"  // 增补：在插入点新增内容

	StatusPending  = "pending"  // 待确认
	StatusAccepted = "accepted" // 已接受
	StatusRejected = "rejected" // 已驳回
	StatusIgnored  = "ignored"  // 已忽略（005：不采纳、折叠可恢复；区别于驳回）

	SourcePage     = "page"     // 任务内已抓页面（1:1）
	SourceText     = "text"     // 自由粘贴文本
	SourceRevision = "revision" // 修订稿派生文档

	// MaxDraftSize caps a single proofread draft (spec A10, 100KB).
	MaxDraftSize = 100 * 1024
	// MaxCardFieldLen caps reason/replacement text (spec A10, 2000 runes).
	MaxCardFieldLen = 2000
)
