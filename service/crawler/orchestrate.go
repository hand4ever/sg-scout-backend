package crawler

import (
	"context"
	"fmt"
	"log"
	"time"

	"sg.scout/model"
	"sg.scout/service/crawler/engine"
)

// Execute runs one queued run to completion (installed as the scheduler
// executor). Dispatch: depth 0 -> single Scrape (US1); depth >= 1 -> site
// crawl (US2, see executeCrawlRun).
func Execute(ctx context.Context, runID uint64) error {
	var run model.CrawlerRun
	if err := model.DB.First(&run, runID).Error; err != nil {
		return err
	}
	var task model.CrawlerTask
	if err := model.DB.First(&task, run.TaskID).Error; err != nil {
		return fmt.Errorf("task %d: %w", run.TaskID, err)
	}
	if run.Status != "running" {
		return fmt.Errorf("run %d not in running state (status=%s)", runID, run.Status)
	}

	eng, err := engineForTask(&task)
	if err != nil {
		_ = RunStopped(runID, err.Error())
		return err
	}

	// Per-task total duration budget (FR-018 timeout_s, default 600s).
	rctx, cancel := context.WithTimeout(ctx, time.Duration(task.TimeoutS)*time.Second)
	defer cancel()

	if task.Depth == 0 {
		return executeScrapeRun(rctx, &task, &run, eng)
	}
	return executeCrawlRun(rctx, &task, &run, eng)
}

// executeScrapeRun handles depth 0: a single Scrape of the entry URL.
func executeScrapeRun(ctx context.Context, task *model.CrawlerTask, run *model.CrawlerRun, eng engine.Engine) error {
	pr, err := eng.Scrape(ctx, task.EntryURL)
	if err != nil {
		_ = RunStopped(run.ID, "引擎调用失败："+err.Error())
		return err
	}
	stats := &RunStats{TotalFound: 1}
	status, err := SavePageResult(task, run.ID, pr, 0)
	if err != nil {
		_ = RunStopped(run.ID, "保存页面失败："+err.Error())
		return err
	}
	switch status {
	case "new":
		stats.PageNew = 1
	case "changed":
		stats.PageChanged = 1
	case "offline":
		stats.PageOffline = 1
	case "failed":
		stats.PageFailed = 1
	}
	if err := RunDone(run.ID, stats); err != nil {
		return err
	}
	_ = SetTaskPageCount(task.ID)
	log.Printf("[crawler] run %d (task %d) scrape done: status=%s", run.ID, task.ID, status)
	return nil
}

// executeCrawlRun handles depth >= 1 via async engine crawl. Full wiring lands
// in US2 (tasks.md T023); until then it stops the run with an explicit error.
func executeCrawlRun(ctx context.Context, task *model.CrawlerTask, run *model.CrawlerRun, eng engine.Engine) error {
	msg := "深度爬取（depth≥1）实现中（US2）"
	_ = RunStopped(run.ID, msg)
	return fmt.Errorf("%s", msg)
}
