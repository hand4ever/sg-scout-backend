package crawler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"sg.scout/model"
	"sg.scout/service/crawler/engine"
	"sg.scout/service/crawler/urlutil"
)

// per-run cancel registry (feature 002 US3 T029): manual stop cancels the
// active run's context, which aborts in-flight fetches and remote jobs.
var (
	runCancelMu sync.Mutex
	runCancels  = map[uint64]context.CancelFunc{}
)

func registerRunCancel(runID uint64, cancel context.CancelFunc) {
	runCancelMu.Lock()
	runCancels[runID] = cancel
	runCancelMu.Unlock()
}

func unregisterRunCancel(runID uint64) {
	runCancelMu.Lock()
	delete(runCancels, runID)
	runCancelMu.Unlock()
}

// cancelRunContext cancels a running run's context if registered.
func cancelRunContext(runID uint64) bool {
	runCancelMu.Lock()
	cancel, ok := runCancels[runID]
	runCancelMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
	return ok
}

// Execute runs one queued run to completion (installed as the scheduler
// executor). Dispatch: depth 0 -> single Scrape (US1); depth >= 1 -> engine
// crawl job (US2/US3 executeCrawlRun). Every run gets its own cancelable
// context so manual stop (FR-026) can abort in-flight work.
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

	rctx, cancel := context.WithCancel(ctx)
	registerRunCancel(runID, cancel)
	defer func() {
		unregisterRunCancel(runID)
		cancel()
	}()

	eng, err := engineForTask(&task)
	if err != nil {
		_ = RunStopped(runID, err.Error())
		return err
	}

	// Per-task total duration budget (FR-018 timeout_s, default 600s).
	tctx, tcancel := context.WithTimeout(rctx, time.Duration(task.TimeoutS)*time.Second)
	defer tcancel()

	if task.Depth == 0 {
		return executeScrapeRun(tctx, &task, &run, eng)
	}
	return executeCrawlRun(tctx, &task, &run, eng)
}

// executeScrapeRun handles depth 0: a single Scrape of the entry URL.
func executeScrapeRun(ctx context.Context, task *model.CrawlerTask, run *model.CrawlerRun, eng engine.Engine) error {
	pr, err := eng.Scrape(ctx, task.EntryURL)
	if err != nil {
		if ctx.Err() != nil {
			msg := "任务被停止"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				msg = "任务超时（timeout_s）"
			}
			_ = RunStopped(run.ID, msg)
			return ctx.Err()
		}
		_ = RunStopped(run.ID, "引擎调用失败："+err.Error())
		return err
	}
	stats := &RunStats{}
	status, err := SavePageResult(task, run.ID, pr, pr.Depth)
	if err != nil {
		_ = RunStopped(run.ID, "保存页面失败："+err.Error())
		return err
	}
	stats.add(status)
	if err := RunDone(run.ID, stats); err != nil {
		return err
	}
	_ = SetTaskPageCount(task.ID)
	log.Printf("[crawler] run %d (task %d) scrape done: status=%s", run.ID, task.ID, status)
	return nil
}

// crawlDelay maps the throttle config to a per-request delay in seconds
// (FR-018: at least throttle_seconds over throttle_pages, min 1s).
func crawlDelay(t *model.CrawlerTask) int {
	pages, secs := t.ThrottlePages, t.ThrottleSeconds
	if pages < 1 {
		pages = 100
	}
	if secs < 1 {
		secs = 60
	}
	d := (secs + pages - 1) / pages
	if d < 1 {
		d = 1
	}
	return d
}

// executeCrawlRun handles depth >= 1 through the engine-neutral job lifecycle
// (feature 002 US3 / research §3): submit -> poll (3s) -> persist batches
// incrementally -> terminal state -> engine errors -> done/stopped.
func executeCrawlRun(ctx context.Context, task *model.CrawlerTask, run *model.CrawlerRun, eng engine.Engine) error {
	req := &engine.CrawlRequest{
		URL:               task.EntryURL,
		MaxDiscoveryDepth: task.Depth,
		AllowSubdomains:   task.IncludeSubdomain,
		AllowHosts:        urlutil.SplitHosts(task.AllowHosts),
		IgnoreRobots:      task.IgnoreRobots,
		IncludeURLs:       urlutil.SplitTokens(task.IncludeURL),
		Limit:             task.PageLimit,
		DelaySeconds:      crawlDelay(task),
	}
	jobID, err := eng.SubmitCrawl(ctx, req)
	if err != nil {
		msg := "引擎提交失败：" + err.Error()
		if ctx.Err() != nil {
			msg = "任务被停止（提交阶段）"
		}
		_ = RunStopped(run.ID, msg)
		return fmt.Errorf("submit crawl: %w", err)
	}
	if err := model.DB.Model(&model.CrawlerRun{}).Where("id = ?", run.ID).
		Update("job_id", jobID).Error; err != nil {
		_ = RunStopped(run.ID, "记录引擎任务失败："+err.Error())
		return err
	}

	stats := &RunStats{}
	persist := func(pr *engine.PageResult) {
		status, serr := SavePageResult(task, run.ID, pr, pr.Depth)
		if serr != nil {
			log.Printf("[crawler] run %d persist page %s failed: %v", run.ID, pr.URL, serr)
			return
		}
		stats.add(status)
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Timeout or manual stop: cancel the engine job best-effort, then
			// mark the run stopped (FR-026; local jobs abort in-flight fetches).
			cancelEngineJob(eng, jobID)
			msg := "任务被停止"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				msg = "任务超时（timeout_s）"
			}
			_ = RunStopped(run.ID, msg)
			return ctx.Err()
		case <-ticker.C:
			batch, perr := eng.PollCrawl(ctx, jobID)
			if perr != nil {
				if ctx.Err() != nil {
					continue // stop path handled by ctx.Done branch
				}
				_ = RunStopped(run.ID, "轮询引擎失败："+perr.Error())
				return perr
			}
			for _, pr := range batch.Pages {
				persist(pr)
			}
			if batch.Status == "completed" || batch.Status == "cancelled" || batch.Status == "failed" {
				return finishCrawlRun(eng, jobID, task, run, stats)
			}
		}
	}
}

// finishCrawlRun ingests engine-level failures then finalizes run stats.
func finishCrawlRun(eng engine.Engine, jobID string, task *model.CrawlerTask, run *model.CrawlerRun, stats *RunStats) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs, ferr := eng.FetchErrors(ctx, jobID)
	if ferr != nil {
		log.Printf("[crawler] run %d fetch errors failed: %v", run.ID, ferr)
	} else {
		for _, pr := range errs {
			status, serr := SavePageResult(task, run.ID, pr, pr.Depth)
			if serr != nil {
				log.Printf("[crawler] run %d record failure %s: %v", run.ID, pr.URL, serr)
				continue
			}
			stats.add(status)
		}
	}
	if err := RunDone(run.ID, stats); err != nil {
		return err
	}
	_ = SetTaskPageCount(task.ID)
	log.Printf("[crawler] run %d (task %d) crawl done: total=%d new=%d changed=%d offline=%d failed=%d",
		run.ID, task.ID, stats.TotalFound, stats.PageNew, stats.PageChanged, stats.PageOffline, stats.PageFailed)
	return nil
}

// cancelEngineJob best-effort cancels an engine job (remote DELETE for cloud,
// in-process cancel for local engines).
func cancelEngineJob(eng engine.Engine, jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := eng.CancelJob(ctx, jobID); err != nil {
		log.Printf("[crawler] cancel engine job %s failed: %v", jobID, err)
	}
}

// add tallies one page outcome into the run stats (any outcome counts
// towards total_found; unchanged included).
func (s *RunStats) add(status string) {
	s.TotalFound++
	switch status {
	case "new":
		s.PageNew++
	case "changed":
		s.PageChanged++
	case "offline":
		s.PageOffline++
	case "failed":
		s.PageFailed++
	}
}
