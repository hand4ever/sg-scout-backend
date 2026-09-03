package crawler

import (
	"context"
	"log"
	"sync"

	"sg.scout/config"
)

// server-level scheduler lifecycle. Concurrency comes from config
// ([crawler].concurrency, FR-019 default 1).

var (
	schedOnce sync.Once
	sched     *Scheduler
	schedStop chan struct{}
)

// StartScheduler launches the crawler run scheduler in the background.
// Executor = Execute (orchestrate.go). Called once from main.
func StartScheduler(ctx context.Context) {
	schedOnce.Do(func() {
		concurrency := config.Cfg.Crawler.Concurrency
		if concurrency < 1 {
			concurrency = 1
		}
		sched = NewScheduler(concurrency, Execute)
		schedStop = make(chan struct{})
		go func() {
			sched.Start(ctx)
			close(schedStop)
		}()
		log.Printf("[crawler] scheduler started (concurrency=%d)", concurrency)
	})
}

// StopScheduler gracefully stops the scheduler (waits current run).
func StopScheduler() {
	if sched != nil {
		sched.Stop()
	}
}
