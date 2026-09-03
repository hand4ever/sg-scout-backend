// Package crawlerentity holds request payloads for the crawler module.
package crawler

// CreateTaskReq is the POST /crawler/tasks body (contracts/api.md).
// Config is locked after creation (FR-025): no update endpoint exists.
type CreateTaskReq struct {
	SourceType       string `json:"source_type"`
	EntryURL         string `json:"entry_url"`
	Depth            *int   `json:"depth"`
	IncludeSubdomain *bool  `json:"include_subdomain"`
	PageLimit        *int   `json:"page_limit"`
	RetryTimes       *int   `json:"retry_times"`
	RetryIntervalS   *int   `json:"retry_interval_s"`
	ThrottlePages    *int   `json:"throttle_pages"`
	ThrottleSeconds  *int   `json:"throttle_seconds"`
	TimeoutS         *int   `json:"timeout_s"`
}

// Defaults mirrors the frontend defaultCreateTaskPayload + research.md A6.
const (
	DefaultDepth          = 0
	DefaultPageLimit      = 10
	DefaultRetryTimes     = 3
	DefaultRetryIntervalS = 2
	DefaultThrottlePages  = 100
	DefaultThrottleSecs   = 60
	DefaultTimeoutS       = 600
)
