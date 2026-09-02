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
	}
}

// InitConfig loads config from the given TOML file path.
// A missing or malformed file falls back to defaults (fail-soft) and logs a notice.
func InitConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("[config] config file not found at %s, using defaults\n", configPath)
			return nil
		}
		return fmt.Errorf("read config file: %w", err)
	}

	if err := toml.Unmarshal(data, Cfg); err != nil {
		fmt.Printf("[config] failed to parse %s: %v, using defaults\n", configPath, err)
		return nil
	}

	fmt.Printf("[config] loaded config from %s\n", configPath)
	return nil
}
