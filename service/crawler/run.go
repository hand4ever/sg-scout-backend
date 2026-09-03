package crawler

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"sg.scout/model"
)

// CreateRun archives a new run (kind crawl|check) for a task and marks the
// task queued. Precondition: task must not already have an active run
// (FR-026/FR-029 conflict rules).
func CreateRun(taskID uint64, kind string) (*model.CrawlerRun, error) {
	var task model.CrawlerTask
	if err := model.DB.First(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: task %d", ErrNotFound, taskID)
		}
		return nil, err
	}
	if kind != "crawl" && kind != "check" {
		return nil, fmt.Errorf("%w: kind 必须为 crawl/check", ErrBadRequest)
	}
	active, err := TaskActive(taskID)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("%w: 任务正在运行/排队中，请先停止", ErrConflict)
	}

	run := &model.CrawlerRun{
		TaskID: taskID,
		Kind:   kind,
		Engine: task.Engine, // snapshot from task (feature 002 FR-003); legacy rows keep their archived value
		Status: "queued",
	}
	if run.Engine == "" {
		run.Engine = "goquery"
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&model.CrawlerTask{}).Where("id = ?", taskID).
			Updates(map[string]any{"status": "queued", "updated_at": &now}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	return run, nil
}

// RunDone finalizes a run as done with its stats and syncs the task status.
func RunDone(runID uint64, stats *RunStats) error {
	now := time.Now()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"status": "done", "finished_at": &now,
			"total_found": stats.TotalFound, "page_new": stats.PageNew,
			"page_changed": stats.PageChanged, "page_offline": stats.PageOffline,
			"page_failed": stats.PageFailed,
		}
		if err := tx.Model(&model.CrawlerRun{}).Where("id = ?", runID).Updates(updates).Error; err != nil {
			return err
		}
		var run model.CrawlerRun
		if err := tx.First(&run, runID).Error; err != nil {
			return err
		}
		return tx.Model(&model.CrawlerTask{}).Where("id = ?", run.TaskID).
			Updates(map[string]any{"status": "done", "updated_at": &now}).Error
	})
}

// RunStopped marks a run/task as stopped (FR-026).
func RunStopped(runID uint64, reason string) error {
	now := time.Now()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": "stopped", "finished_at": &now}
		if reason != "" {
			updates["error_msg"] = reason
		}
		if err := tx.Model(&model.CrawlerRun{}).Where("id = ?", runID).Updates(updates).Error; err != nil {
			return err
		}
		var run model.CrawlerRun
		if err := tx.First(&run, runID).Error; err != nil {
			return err
		}
		return tx.Model(&model.CrawlerTask{}).Where("id = ?", run.TaskID).
			Updates(map[string]any{"status": "stopped", "updated_at": &now}).Error
	})
}

// RunStats aggregates one run's outcome counts.
type RunStats struct {
	TotalFound  int
	PageNew     int
	PageChanged int
	PageOffline int
	PageFailed  int
}

// RunDetailView is the GET /crawler/runs/{id} payload.
type RunDetailView struct {
	Run     *RunView      `json:"run"`
	Pages   []RunPageItem `json:"pages"`
	Summary struct {
		Changed   int `json:"changed"`
		Unchanged int `json:"unchanged"`
		Offline   int `json:"offline"`
		Failed    int `json:"failed"`
		New       int `json:"new"`
	} `json:"summary"`
}

// RunPageItem is one page outcome row within a run detail.
type RunPageItem struct {
	PageID    uint64     `json:"page_id"`
	URL       string     `json:"url"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	CrawledAt *time.Time `json:"crawled_at"`
	Depth     int        `json:"depth"`
}

// GetRunDetail loads a run with its per-page outcomes (latest run page list).
func GetRunDetail(runID uint64) (*RunDetailView, error) {
	var run model.CrawlerRun
	if err := model.DB.First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: run %d", ErrNotFound, runID)
		}
		return nil, err
	}
	dv := &RunDetailView{Run: runView(&run), Pages: []RunPageItem{}}
	var rows []struct {
		model.RunPage
		URL   string `gorm:"column:url"`
		Title string `gorm:"column:title"`
		Depth int    `gorm:"column:depth"`
	}
	err := model.DB.Table("run_page").
		Select("run_page.*, crawler_page.url, crawler_page.title, crawler_page.depth").
		Joins("JOIN crawler_page ON crawler_page.id = run_page.page_id").
		Where("run_page.run_id = ?", runID).
		Order("run_page.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		dv.Pages = append(dv.Pages, RunPageItem{
			PageID: row.PageID, URL: row.URL, Title: row.Title,
			Status: row.Status, Error: row.Error,
			CrawledAt: row.CrawledAt, Depth: row.Depth,
		})
		switch row.Status {
		case "changed":
			dv.Summary.Changed++
		case "unchanged":
			dv.Summary.Unchanged++
		case "offline":
			dv.Summary.Offline++
		case "failed":
			dv.Summary.Failed++
		case "new":
			dv.Summary.New++
		}
	}
	return dv, nil
}

// FailedPageURLs returns the failed page URLs of a run (US4 retry source).
func FailedPageURLs(runID uint64) ([]model.CrawlerPage, error) {
	var pages []model.CrawlerPage
	err := model.DB.Table("crawler_page").
		Select("crawler_page.*").
		Joins("JOIN run_page ON run_page.page_id = crawler_page.id").
		Where("run_page.run_id = ? AND run_page.status = ?", runID, "failed").
		Scan(&pages).Error
	return pages, err
}
