// Package proofread implements the text proofreading workbench module
// (feature 004-text-proofreading; contracts/api.md).
package proofread

import "errors"

// Sentinel errors surfaced through the unified envelope (Chinese messages).
var (
	// ErrBadRequest covers invalid params, anchor ranges, overlap, etc.
	ErrBadRequest = errors.New("参数错误")
	// ErrNotFound covers missing documents/cards/pages.
	ErrNotFound = errors.New("资源不存在")
	// ErrConflict covers duplicate page docs and already-latest upgrades.
	ErrConflict = errors.New("状态冲突")
)
