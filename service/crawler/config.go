package crawler

import (
	"fmt"
	"strings"
	"time"

	"sg.scout/config"
	"sg.scout/model"
	"sg.scout/service/crawler/engine"
)

// retryForTask maps task-level retry settings (FR-023: engine call layer).
func retryForTask(t *model.CrawlerTask) (int, time.Duration) {
	retries := t.RetryTimes
	if retries < 0 {
		retries = 0
	}
	wait := time.Duration(t.RetryIntervalS) * time.Second
	if wait <= 0 {
		wait = 2 * time.Second
	}
	return retries, wait
}

// engineForTask builds the crawl engine for a task from its archived engine
// snapshot (feature 002 FR-003). Connection params come from per-engine
// config.toml sections; secrets stay in the config file (FR-012).
func engineForTask(t *model.CrawlerTask) (engine.Engine, error) {
	retries, wait := retryForTask(t)
	ec := config.Cfg.Crawler.Engine
	switch t.Engine {
	case "", engine.EngineGoquery:
		// Local direct-fetch engine: in-process, no external service (D2).
		return engine.NewGoquery(retries, wait), nil
	case engine.EngineFirecrawl:
		cloud := ec.FirecrawlBaseURL() == "" || strings.HasPrefix(ec.FirecrawlBaseURL(), "https://api.firecrawl.dev")
		if cloud && ec.FirecrawlAPIKey() == "" {
			return nil, fmt.Errorf("%w: Firecrawl 云需 api_key（config.toml [crawler.engine.firecrawl]）", ErrEngineConfig)
		}
		return engine.New(ec.FirecrawlBaseURL(), ec.FirecrawlAPIKey(), retries, wait), nil
	case engine.EngineCrawl4AI:
		conn := ec.Crawl4AI
		if conn.BaseURL == "" {
			conn.BaseURL = "http://127.0.0.1:11235"
		}
		if conn.APIToken == "" {
			return nil, fmt.Errorf("%w: crawl4ai 需 api_token（config.toml [crawler.engine.crawl4ai]）", ErrEngineConfig)
		}
		return engine.NewCrawl4AI(conn.BaseURL, conn.APIToken, retries, wait), nil
	default:
		return nil, fmt.Errorf("%w: provider=%q", ErrEngineConfig, t.Engine)
	}
}
