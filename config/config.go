package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds all configuration sections.
type Config struct {
	App      AppConfig      `toml:"app"`
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	Crawler  CrawlerConfig  `toml:"crawler"`
}

// CrawlerConfig holds crawler module settings (concurrency + engine).
type CrawlerConfig struct {
	Concurrency int          `toml:"concurrency"`
	StorageRoot string       `toml:"storage_root"`
	Engine      EngineConfig `toml:"engine"`
}

// EngineConfig holds the crawl engine connection settings.
// provider/based on research.md rev2: Firecrawl v2 API (cloud by default).
type EngineConfig struct {
	Provider string `toml:"provider"`
	BaseURL  string `toml:"base_url"`
	APIKey   string `toml:"api_key"`
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
				Provider: "firecrawl",
				BaseURL:  "https://api.firecrawl.dev",
				APIKey:   "",
			},
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
