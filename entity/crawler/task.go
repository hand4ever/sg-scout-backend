// Package crawlerentity holds request payloads for the crawler module.
package crawler

// CreateTaskReq is the POST /crawler/tasks body (contracts/api.md).
// Config is locked after creation (FR-025): no update endpoint exists.
type CreateTaskReq struct {
	SourceType       string `json:"source_type"`
	Engine           string `json:"engine"` // optional; empty = global default_engine (feature 002 FR-003)
	EntryURL         string `json:"entry_url"`
	Depth            *int   `json:"depth"`
	IncludeSubdomain *bool  `json:"include_subdomain"`
	AllowHosts       string `json:"allow_hosts"` // comma-separated external hosts to follow ("" = same-site only)
	RespectRobots    *bool  `json:"respect_robots"`
	IncludeURL       string `json:"include_url"`  // comma-separated URL substrings; only links containing one are followed
	ContentMode      string `json:"content_mode"` // main (default) | full
	PageLimit        *int   `json:"page_limit"`
	RetryTimes       *int   `json:"retry_times"`
	RetryIntervalS   *int   `json:"retry_interval_s"`
	ThrottlePages    *int   `json:"throttle_pages"`
	ThrottleSeconds  *int   `json:"throttle_seconds"`
	TimeoutS         *int   `json:"timeout_s"`
}

// Defaults mirrors the frontend defaultCreateTaskPayload + research.md A6.
const (
	DefaultEngine         = "goquery" // feature 002 D2: self-hosted direct engine is the default
	DefaultDepth          = 0
	DefaultPageLimit      = 10
	DefaultRetryTimes     = 3
	DefaultRetryIntervalS = 2
	DefaultThrottlePages  = 100
	DefaultThrottleSecs   = 60
	DefaultTimeoutS       = 600
)
