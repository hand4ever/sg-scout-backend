package crawler

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
	"sg.scout/model"
)

// Executor runs one queued run (kind crawl/check). It is installed by the
// orchestrator package at startup; scheduler stays agnostic of orchestration.
type Executor func(ctx context.Context, runID uint64) error

// Scheduler consumes queued crawler runs with fixed concurrency (FR-019:
// system config, default 1). Run state transitions happen in the DB so the
// queue survives restarts: queued -> running (worker) -> done/stopped.
type Scheduler struct {
	concurrency int
	pollEvery   time.Duration
	exec        Executor
	stopCh      chan struct{}
	doneCh      chan struct{}
}

// NewScheduler creates a scheduler; exec is required (set by orchestrator).
func NewScheduler(concurrency int, exec Executor) *Scheduler {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Scheduler{
		concurrency: concurrency,
		pollEvery:   2 * time.Second,
		exec:        exec,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

// Start launches workers and blocks until Stop is called.
func (s *Scheduler) Start(ctx context.Context) {
	defer close(s.doneCh)
	for i := 0; i < s.concurrency; i++ {
		go s.worker(ctx)
	}
	<-s.stopCh
	log.Printf("[crawler/scheduler] stopping (concurrency=%d)", s.concurrency)
}

// Stop signals workers to finish the current run and exit.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

func (s *Scheduler) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}
		run := s.dequeueQueued()
		if run == nil {
			time.Sleep(s.pollEvery)
			continue
		}
		log.Printf("[crawler/scheduler] run %d start (task=%d kind=%s)", run.ID, run.TaskID, run.Kind)
		err := s.exec(ctx, run.ID)
		if err != nil {
			log.Printf("[crawler/scheduler] run %d finished with error: %v", run.ID, err)
		} else {
			log.Printf("[crawler/scheduler] run %d done", run.ID)
		}
	}
}

// dequeueQueued atomically claims the oldest queued run: marks it running and
// returns it. Returns nil when the queue is empty.
func (s *Scheduler) dequeueQueued() *model.CrawlerRun {
	var run model.CrawlerRun
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status = ?", "queued").Order("created_at ASC, id ASC").
			First(&run).Error; err != nil {
			return err // gorm.ErrRecordNotFound -> no work
		}
		now := time.Now()
		return tx.Model(&model.CrawlerRun{}).Where("id = ?", run.ID).
			Updates(map[string]any{"status": "running", "started_at": &now}).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		log.Printf("[crawler/scheduler] dequeue failed: %v", err)
		return nil
	}
	return &run
}
