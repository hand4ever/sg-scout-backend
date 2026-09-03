package engine

import "sg.scout/config"

// Engine identifiers (feature 002 FR-001 registry; values persisted in
// crawler_task.engine / crawler_run.engine / system_settings.default_engine).
const (
	EngineFirecrawl = "firecrawl"
	EngineCrawl4AI  = "crawl4ai"
	EngineGoquery   = "goquery"
)

// Capabilities declares what an engine can do (feature 002 FR-015/A6:
// capability differences are explicit and validated, never silently degraded).
type Capabilities struct {
	SupportsDepth bool   `json:"supports_depth"`
	JSRender      bool   `json:"js_render"`
	ExitNetwork   string `json:"exit_network"` // local | cloud
}

// Info describes one registered engine for GET /crawler/engines.
type Info struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Capabilities Capabilities `json:"capabilities"`
	Configured   bool         `json:"configured"`
	Available    bool         `json:"available"`
}

// staticInfo returns the static metadata of a registered engine.
func staticInfo(id string) (Info, bool) {
	switch id {
	case EngineFirecrawl:
		return Info{
			ID: EngineFirecrawl, Name: "Firecrawl 云", Description: "云端抓取服务（境外节点，需 api_key）",
			Capabilities: Capabilities{SupportsDepth: true, JSRender: true, ExitNetwork: "cloud"},
		}, true
	case EngineCrawl4AI:
		return Info{
			ID: EngineCrawl4AI, Name: "Crawl4AI 本地", Description: "本地浏览器渲染抓取（Docker 服务，research.md §5）",
			Capabilities: Capabilities{SupportsDepth: true, JSRender: true, ExitNetwork: "local"},
		}, true
	case EngineGoquery:
		return Info{
			ID: EngineGoquery, Name: "goquery 本地直连", Description: "本地直连抓取（无 JS 渲染、零外部依赖，默认保底）",
			Capabilities: Capabilities{SupportsDepth: true, JSRender: false, ExitNetwork: "local"},
		}, true
	}
	return Info{}, false
}

// Lookup reports whether id is a registered engine.
func Lookup(id string) bool {
	_, ok := staticInfo(id)
	return ok
}

// firecrawlConfigured mirrors the runtime requirement: cloud needs an api_key;
// a self-hosted base URL without a key is acceptable (research.md §11).
func firecrawlConfigured() bool {
	ec := config.Cfg.Crawler.Engine
	if ec.FirecrawlAPIKey() != "" {
		return true
	}
	base := ec.FirecrawlBaseURL()
	return base != "" && base != "https://api.firecrawl.dev"
}

// crawl4aiConfigured requires base_url + api_token (0.9.x security default).
func crawl4aiConfigured() bool {
	ec := config.Cfg.Crawler.Engine.Crawl4AI
	return ec.BaseURL != "" && ec.APIToken != ""
}

// List returns all registered engines with runtime configured/available state.
// Available is a light check (deep health probe for crawl4ai is wired in its
// adapter, feature 002 US2); goquery is in-process so always available.
func List() []Info {
	out := make([]Info, 0, 3)
	for _, id := range []string{EngineFirecrawl, EngineCrawl4AI, EngineGoquery} {
		info, _ := staticInfo(id)
		switch id {
		case EngineFirecrawl:
			info.Configured = firecrawlConfigured()
			info.Available = info.Configured
		case EngineCrawl4AI:
			info.Configured = crawl4aiConfigured()
			info.Available = info.Configured && Crawl4AIProbePing() // live health probe (US2)
		case EngineGoquery:
			info.Configured = true
			info.Available = true
		}
		out = append(out, info)
	}
	return out
}
