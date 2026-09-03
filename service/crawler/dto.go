package crawler

import (
	"time"

	"sg.scout/model"
)

// TaskView is the API task payload: task fields + optional latest run summary.
type TaskView struct {
	ID               uint64     `json:"id"`
	SourceType       string     `json:"source_type"`
	EntryURL         string     `json:"entry_url"`
	Depth            int        `json:"depth"`
	IncludeSubdomain bool       `json:"include_subdomain"`
	PageLimit        int        `json:"page_limit"`
	RetryTimes       int        `json:"retry_times"`
	RetryIntervalS   int        `json:"retry_interval_s"`
	ThrottlePages    int        `json:"throttle_pages"`
	ThrottleSeconds  int        `json:"throttle_seconds"`
	TimeoutS         int        `json:"timeout_s"`
	Status           string     `json:"status"`
	PageCount        int        `json:"page_count"`
	LastRunAt        *time.Time `json:"last_run_at"`
	CreatedAt        time.Time  `json:"created_at"`
	LastRun          *RunView   `json:"last_run,omitempty"`
}

// RunView is the run summary payload (crawler_run row, no page list).
type RunView struct {
	ID         uint64     `json:"id"`
	TaskID     uint64     `json:"task_id"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	TotalFound int        `json:"total_found"`
	PageNew    int        `json:"page_new"`
	PageChanged int       `json:"page_changed"`
	PageOffline int       `json:"page_offline"`
	PageFailed  int       `json:"page_failed"`
	ErrorMsg   string     `json:"error_msg"`
	CreatedAt  time.Time  `json:"created_at"`
}

// taskView builds a TaskView from a task row.
func taskView(t *model.CrawlerTask) *TaskView {
	return &TaskView{
		ID:               t.ID,
		SourceType:       t.SourceType,
		EntryURL:         t.EntryURL,
		Depth:            t.Depth,
		IncludeSubdomain: t.IncludeSubdomain,
		PageLimit:        t.PageLimit,
		RetryTimes:       t.RetryTimes,
		RetryIntervalS:   t.RetryIntervalS,
		ThrottlePages:    t.ThrottlePages,
		ThrottleSeconds:  t.ThrottleSeconds,
		TimeoutS:         t.TimeoutS,
		Status:           t.Status,
		PageCount:        t.PageCount,
		LastRunAt:        t.LastRunAt,
		CreatedAt:        t.CreatedAt,
	}
}

// runView builds a RunView from a run row.
func runView(r *model.CrawlerRun) *RunView {
	return &RunView{
		ID:          r.ID,
		TaskID:      r.TaskID,
		Kind:        r.Kind,
		Status:      r.Status,
		StartedAt:   r.StartedAt,
		FinishedAt:  r.FinishedAt,
		TotalFound:  r.TotalFound,
		PageNew:     r.PageNew,
		PageChanged: r.PageChanged,
		PageOffline: r.PageOffline,
		PageFailed:  r.PageFailed,
		ErrorMsg:    r.ErrorMsg,
		CreatedAt:   r.CreatedAt,
	}
}
