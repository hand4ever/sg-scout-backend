package crawler

import (
	"fmt"
	"strings"
	"time"

	"sg.scout/config"
	"sg.scout/model"
	"sg.scout/service/crawler/engine"
)

// concurrencyOrDefault returns the scheduler concurrency (FR-019, default 1).
func concurrencyOrDefault() int {
	if c := config.Cfg.Crawler.Concurrency; c > 0 {
		return c
	}
	return 1
}

func engineBaseURL() string {
	return config.Cfg.Crawler.Engine.BaseURL
}

func configAppVersion() string {
	return config.Cfg.App.Version
}

// engineForTask builds the crawl engine for a task from global config
// ([crawler.engine]) plus task-level retry settings (FR-023 maps to API-layer
// retries; research.md rev2 §4).
func engineForTask(t *model.CrawlerTask) (engine.Engine, error) {
	ec := config.Cfg.Crawler.Engine
	if ec.Provider == "" {
		ec.Provider = "firecrawl"
	}
	if ec.Provider != "firecrawl" {
		return nil, fmt.Errorf("%w: provider=%q", ErrEngineConfig, ec.Provider)
	}
	cloud := ec.BaseURL == "" || strings.HasPrefix(ec.BaseURL, "https://api.firecrawl.dev")
	if cloud && ec.APIKey == "" {
		return nil, fmt.Errorf("%w: Firecrawl 云需 api_key（config.toml [crawler.engine]）", ErrEngineConfig)
	}
	retries := t.RetryTimes
	if retries < 0 {
		retries = 0
	}
	wait := time.Duration(t.RetryIntervalS) * time.Second
	if wait <= 0 {
		wait = 2 * time.Second
	}
	return engine.New(ec.BaseURL, ec.APIKey, retries, wait), nil
}
