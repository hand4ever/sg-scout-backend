package crawler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"sg.scout/model"
)

// Task control endpoints (feature 002 US3: engine-neutral completion of the
// 001-pending task lifecycle — check/stop/delete/retry/version/delete-page).

// CheckTaskRun archives a check run (FR-007/014). Execution semantics are
// shared with crawl runs (Execute re-fetches and compares fingerprints).
func CheckTaskRun(taskID uint64) (*RunView, error) {
	run, err := CreateRun(taskID, "check")
	if err != nil {
		return nil, err
	}
	return runView(run), nil
}

// ActiveRun returns the queued/running run of a task, if any.
func ActiveRun(taskID uint64) (*model.CrawlerRun, error) {
	var run model.CrawlerRun
	err := model.DB.Where("task_id = ? AND status IN ?", taskID, []string{"queued", "running"}).
		Order("id DESC").First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 任务当前没有运行中/排队中的执行", ErrConflict)
		}
		return nil, err
	}
	return &run, nil
}

// StopActiveRun stops the queued/running execution of a task (FR-026):
// running runs are cancelled through the per-run context (aborts in-flight
// fetches and engine jobs); queued runs are marked stopped directly.
func StopActiveRun(taskID uint64) (*RunView, error) {
	run, err := ActiveRun(taskID)
	if err != nil {
		return nil, err
	}
	if run.Status == "running" {
		cancelRunContext(run.ID)
		// Execute finalizes the stop; wait briefly so the view is consistent.
		for i := 0; i < 30; i++ {
			var cur model.CrawlerRun
			if err := model.DB.First(&cur, run.ID).Error; err != nil {
				return nil, err
			}
			if cur.Status != "running" {
				run = &cur
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		if err := RunStopped(run.ID, "用户手动停止（排队中）"); err != nil {
			return nil, err
		}
	}
	var final model.CrawlerRun
	if err := model.DB.First(&final, run.ID).Error; err != nil {
		return nil, err
	}
	return runView(&final), nil
}

// DeleteTask cascade-deletes a task: run_page/page_version/crawler_run/
// crawler_page/crawler_task rows + artifact directory (FR-021).
func DeleteTask(taskID uint64) error {
	var task model.CrawlerTask
	if err := model.DB.First(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: task %d", ErrNotFound, taskID)
		}
		return err
	}
	active, err := TaskActive(taskID)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: 任务正在运行/排队中，请先停止", ErrConflict)
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE rp FROM run_page rp JOIN crawler_run r ON r.id = rp.run_id WHERE r.task_id = ?`, taskID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE pv FROM page_version pv JOIN crawler_page p ON p.id = pv.page_id WHERE p.task_id = ?`, taskID).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", taskID).Delete(&model.CrawlerRun{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", taskID).Delete(&model.CrawlerPage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", taskID).Delete(&model.CrawlerTask{}).Error
	})
	if err != nil {
		return fmt.Errorf("delete task %d: %w", taskID, err)
	}
	if err := RemoveTaskDir(taskID); err != nil {
		return fmt.Errorf("remove task dir: %w", err)
	}
	return nil
}

// RetryFailedRun re-fetches the failed pages of one run in place (US4/001
// decision: same-run catch-up, run_page statuses updated, stats recomputed).
func RetryFailedRun(runID uint64) (*RunView, error) {
	var run model.CrawlerRun
	if err := model.DB.First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: run %d", ErrNotFound, runID)
		}
		return nil, err
	}
	var task model.CrawlerTask
	if err := model.DB.First(&task, run.TaskID).Error; err != nil {
		return nil, err
	}
	if run.Status == "queued" || run.Status == "running" {
		return nil, fmt.Errorf("%w: 该执行正在运行中，无法重试失败页", ErrConflict)
	}
	pages, err := FailedPageURLs(runID)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: 该执行没有失败页可重试", ErrBadRequest)
	}
	eng, err := engineForTask(&task)
	if err != nil {
		return nil, err
	}
	okCount := 0
	for _, page := range pages {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		pr, serr := eng.Scrape(ctx, page.URL)
		cancel()
		if serr != nil {
			continue // keep original failure state
		}
		pr.Depth = page.Depth
		status, serr := SavePageResult(&task, run.ID, pr, pr.Depth)
		if serr == nil && status != "failed" {
			okCount++ // recovered only when the page now yields content/offline signal
		}
	}
	if err := refreshRunStats(runID); err != nil {
		return nil, err
	}
	var final model.CrawlerRun
	if err := model.DB.First(&final, runID).Error; err != nil {
		return nil, err
	}
	view := runView(&final)
	view.ErrorMsg = fmt.Sprintf("失败页重试完成：%d/%d 成功", okCount, len(pages))
	return view, nil
}

// refreshRunStats recomputes a run's counter columns from its run_page rows.
func refreshRunStats(runID uint64) error {
	type cnt struct {
		Status string
		N      int64
	}
	var rows []cnt
	if err := model.DB.Table("run_page").Select("status, COUNT(*) AS n").
		Where("run_id = ?", runID).Group("status").Scan(&rows).Error; err != nil {
		return err
	}
	updates := map[string]any{"total_found": int64(0), "page_new": 0, "page_changed": 0, "page_offline": 0, "page_failed": 0}
	total := int64(0)
	for _, r := range rows {
		total += r.N
		switch r.Status {
		case "new":
			updates["page_new"] = r.N
		case "changed":
			updates["page_changed"] = r.N
		case "offline":
			updates["page_offline"] = r.N
		case "failed":
			updates["page_failed"] = r.N
		}
	}
	updates["total_found"] = total
	return model.DB.Model(&model.CrawlerRun{}).Where("id = ?", runID).Updates(updates).Error
}

// DeletePage removes one page: version chain + run_page rows + page row +
// artifact directory (FR-021 granularity ①).
func DeletePage(pageID uint64) error {
	var page model.CrawlerPage
	if err := model.DB.First(&page, pageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: page %d", ErrNotFound, pageID)
		}
		return err
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("page_id = ?", pageID).Delete(&model.PageVersion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("page_id = ?", pageID).Delete(&model.RunPage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", pageID).Delete(&model.CrawlerPage{}).Error
	})
	if err != nil {
		return fmt.Errorf("delete page %d: %w", pageID, err)
	}
	if err := RemovePageDir(page.TaskID, pageID); err != nil {
		return fmt.Errorf("remove page dir: %w", err)
	}
	return nil
}

// GetPageVersionContent returns one version's meta + body markdown.
func GetPageVersionContent(pageID uint64, version int) (*PageVersionView, error) {
	var pv model.PageVersion
	if err := model.DB.Where("page_id = ? AND version = ?", pageID, version).First(&pv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: 版本 %d 不存在", ErrNotFound, version)
		}
		return nil, err
	}
	var page model.CrawlerPage
	if err := model.DB.First(&page, pv.PageID).Error; err != nil {
		return nil, err
	}
	content, err := ReadVersionContent(page.TaskID, pageID, version)
	if err != nil {
		return nil, err
	}
	return &PageVersionView{
		Version: pv.Version, CrawledAt: pv.CrawledAt, Kind: pv.Kind, Content: content,
	}, nil
}

// PageVersionView is the version content payload.
type PageVersionView struct {
	Version   int       `json:"version"`
	CrawledAt time.Time `json:"crawled_at"`
	Kind      string    `json:"kind"`
	Content   string    `json:"content"`
}
