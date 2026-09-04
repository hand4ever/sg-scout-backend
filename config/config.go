package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds all configuration sections.
type Config struct {
	App       AppConfig       `toml:"app"`
	Server    ServerConfig    `toml:"server"`
	Database  DatabaseConfig  `toml:"database"`
	Crawler   CrawlerConfig   `toml:"crawler"`
	Proofread ProofreadConfig `toml:"proofread"`
}

// ProofreadConfig holds auto-proofreading engine connection settings
// (feature 005). Secrets (provider base_url/api_key) live ONLY in this file —
// never in the DB (research D3). Providers map is keyed by a user-chosen name
// that engine instances reference (e.g. deepseek / local-ollama).
type ProofreadConfig struct {
	Providers map[string]ProofreadProvider `toml:"providers"`
}

// ProofreadProvider is one LLM / third-party API connection (OpenAI-compatible
// chat/completions for llm engines; contracts appendix A for httpapi engines).
type ProofreadProvider struct {
	BaseURL  string `toml:"base_url"`
	APIKey   string `toml:"api_key"`
	TimeoutS int    `toml:"timeout_s"` // default 120 when 0 (spec A6)
}

// CrawlerConfig holds crawler module settings (concurrency + engine).
type CrawlerConfig struct {
	Concurrency int          `toml:"concurrency"`
	StorageRoot string       `toml:"storage_root"`
	Engine      EngineConfig `toml:"engine"`
}

// EngineConfig holds engine selection + per-engine connection sections.
// Provider is the default-engine seed source for system_settings (feature 002);
// legacy flat base_url/api_key are kept as firecrawl fallback for old config.toml.
type EngineConfig struct {
	Provider  string     `toml:"provider"`
	BaseURL   string     `toml:"base_url"`
	APIKey    string     `toml:"api_key"`
	Firecrawl EngineConn `toml:"firecrawl"`
	Crawl4AI  EngineConn `toml:"crawl4ai"`
}

// EngineConn is one engine's connection settings (secret fields stay in the
// config file / env — never stored in DB, feature 002 FR-012).
type EngineConn struct {
	BaseURL  string `toml:"base_url"`
	APIKey   string `toml:"api_key"`
	APIToken string `toml:"api_token"`
}

// FirecrawlBaseURL prefers the sub-section, falling back to the legacy flat value.
func (e EngineConfig) FirecrawlBaseURL() string {
	if e.Firecrawl.BaseURL != "" {
		return e.Firecrawl.BaseURL
	}
	return e.BaseURL
}

// FirecrawlAPIKey prefers the sub-section, falling back to the legacy flat value.
func (e EngineConfig) FirecrawlAPIKey() string {
	if e.Firecrawl.APIKey != "" {
		return e.Firecrawl.APIKey
	}
	return e.APIKey
}

// AppConfig holds application metadata.
type AppConfig struct {
	Name      string `toml:"name"`
	Version   string `toml:"version"`
	BuildTime string `toml:"build_time"`
}

// ServerConfig holds server settings.
type ServerConfig struct {
	Port string `toml:"port"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	MySQL MySQLConfig `toml:"mysql"`
}

// MySQLConfig holds the MySQL DSN.
type MySQLConfig struct {
	DSN string `toml:"dsn"`
}

// Cfg is the global config instance, initialized at startup.
var Cfg = defaultConfig()

// Default returns a fresh copy of the built-in defaults (tests use it to
// restore the global after mutation).
func Default() *Config {
	return defaultConfig()
}

// defaultConfig returns safe defaults when config file is missing or invalid.
func defaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:      "SG Scout",
			Version:   "0.1.0",
			BuildTime: "unknown",
		},
		Server: ServerConfig{
			Port: ":1324",
		},
		Database: DatabaseConfig{
			MySQL: MySQLConfig{
				DSN: "root:password@tcp(127.0.0.1:3306)/sg_scout?charset=utf8mb4&parseTime=True&loc=Local",
			},
		},
		Crawler: CrawlerConfig{
			Concurrency: 1,
			StorageRoot: "./data",
			Engine: EngineConfig{
				Provider: "",
				BaseURL:  "https://api.firecrawl.dev",
			},
		},
		Proofread: ProofreadConfig{
			Providers: map[string]ProofreadProvider{},
		},
	}
}

// InitConfig loads config from the given TOML file path.
// Strict by Constitution VI (fail fast): a missing or malformed config file
// MUST return an error so the caller can terminate with a non-zero exit.
// Cfg keeps safe defaults only as pre-load state; they are NOT a fallback path.
func InitConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("config file not found at %s (constitution VI: fail fast)", configPath)
	}

	if err := toml.Unmarshal(data, Cfg); err != nil {
		return fmt.Errorf("parse config file %s: %w", configPath, err)
	}

	fmt.Printf("[config] loaded config from %s\n", configPath)
	return nil
}
