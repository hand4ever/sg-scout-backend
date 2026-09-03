package model

import "time"

// Table names for the crawler module (data-model.md). Schema is created by the
// user via schema/crawler.sql (Constitution VII: no AutoMigrate).

// CrawlerTask is a crawl/monitor task holding locked configuration.
type CrawlerTask struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SourceType       string     `gorm:"size:32;not null;default:web" json:"source_type"`
	Engine           string     `gorm:"size:16;not null;default:goquery" json:"engine"` // archived engine snapshot (feature 002 FR-003)
	EntryURL         string     `gorm:"size:2048;not null" json:"entry_url"`
	EntryURLKey      string     `gorm:"size:64;not null" json:"-"`
	Depth            int        `gorm:"not null;default:0" json:"depth"`
	IncludeSubdomain bool       `gorm:"not null;default:false" json:"include_subdomain"`
	AllowHosts       string     `gorm:"size:512;not null;default:''" json:"allow_hosts"`   // comma-separated external hosts allowed past the same-site boundary (feature 002 FR-0XX)
	IgnoreRobots     bool       `gorm:"not null;default:false" json:"-"`                   // true = fetch robots-disallowed paths (wechat). API surfaces the inverse as respect_robots.
	IncludeURL       string     `gorm:"size:1024;not null;default:''" json:"include_url"`  // comma-separated URL substrings; only links containing one are followed (entry always fetched)
	ContentMode      string     `gorm:"size:16;not null;default:main" json:"content_mode"` // main = article-only (readability); full = whole page
	PageLimit        int        `gorm:"not null;default:10" json:"page_limit"`
	RetryTimes       int        `gorm:"not null;default:3" json:"retry_times"`
	RetryIntervalS   int        `gorm:"not null;default:2" json:"retry_interval_s"`
	ThrottlePages    int        `gorm:"not null;default:100" json:"throttle_pages"`
	ThrottleSeconds  int        `gorm:"not null;default:60" json:"throttle_seconds"`
	TimeoutS         int        `gorm:"not null;default:600" json:"timeout_s"`
	Status           string     `gorm:"size:16;not null;default:pending;index" json:"status"`
	PageCount        int        `gorm:"not null;default:0" json:"page_count"`
	LastRunAt        *time.Time `json:"last_run_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (CrawlerTask) TableName() string { return "crawler_task" }

// CrawlerRun is one full execution (crawl or check) of a task.
type CrawlerRun struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      uint64     `gorm:"not null;index:idx_run_task_created,priority:1" json:"task_id"`
	Kind        string     `gorm:"size:8;not null" json:"kind"`                    // crawl | check
	Engine      string     `gorm:"size:16;not null;default:goquery" json:"engine"` // engine snapshot from task (feature 002)
	JobID       string     `gorm:"size:64" json:"job_id"`
	Status      string     `gorm:"size:16;not null;default:queued;index" json:"status"` // queued|running|stopped|done
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	TotalFound  int        `gorm:"not null;default:0" json:"total_found"`
	PageNew     int        `gorm:"not null;default:0" json:"page_new"`
	PageChanged int        `gorm:"not null;default:0" json:"page_changed"`
	PageOffline int        `gorm:"not null;default:0" json:"page_offline"`
	PageFailed  int        `gorm:"not null;default:0" json:"page_failed"`
	ErrorMsg    string     `gorm:"size:1024" json:"error_msg"`
	CreatedAt   time.Time  `json:"created_at"`

	// Run summary alias for API responses (computed by service, not a column).
	// gorm:"-" keeps it out of the schema.
}

func (CrawlerRun) TableName() string { return "crawler_run" }

// CrawlerPage is one crawled page, unique per (task, normalized url key).
type CrawlerPage struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID            uint64    `gorm:"not null;index" json:"task_id"`
	URL               string    `gorm:"size:2048;not null" json:"url"`
	URLKey            string    `gorm:"size:64;not null" json:"-"` // unique per task enforced by schema uk_task_urlkey
	Depth             int       `gorm:"not null;default:0" json:"depth"`
	Title             string    `gorm:"size:1024;not null;default:''" json:"title"`
	LatestVersion     int       `gorm:"not null;default:0" json:"latest_version"`
	LatestFingerprint string    `gorm:"size:64;not null;default:''" json:"latest_fingerprint"`
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	CreatedAt         time.Time `json:"created_at"`
}

func (CrawlerPage) TableName() string { return "crawler_page" }

// PageVersion is one content version of a page (v1 starts at first fetch).
type PageVersion struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	PageID      uint64    `gorm:"not null;uniqueIndex:uk_page_version,priority:1" json:"page_id"`
	Version     int       `gorm:"not null;uniqueIndex:uk_page_version,priority:2" json:"version"`
	RunID       uint64    `gorm:"not null;index" json:"run_id"`
	Kind        string    `gorm:"size:8;not null" json:"kind"` // first | change
	Fingerprint string    `gorm:"size:64;not null" json:"fingerprint"`
	CharCount   int       `gorm:"not null;default:0" json:"char_count"`
	CrawledAt   time.Time `json:"crawled_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (PageVersion) TableName() string { return "page_version" }

// RunPage is the per-page outcome of one run (FR-008 statuses).
type RunPage struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	RunID     uint64     `gorm:"not null;uniqueIndex:uk_run_page,priority:1" json:"run_id"`
	PageID    uint64     `gorm:"not null;uniqueIndex:uk_run_page,priority:2" json:"page_id"`
	Status    string     `gorm:"size:16;not null;index:idx_run_status,priority:1" json:"status"` // new|unchanged|changed|offline|failed
	Error     string     `gorm:"size:512" json:"error"`
	CrawledAt *time.Time `json:"crawled_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (RunPage) TableName() string { return "run_page" }
