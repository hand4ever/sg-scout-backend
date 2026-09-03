package main

import (
	"context"
	"net/http"
	"os"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"sg.scout/config"
	"sg.scout/middle"
	"sg.scout/model"
	"sg.scout/router"
	"sg.scout/service/crawler"
	"sg.scout/service/settings"
)

// corsConfig defines the CORS middleware configuration.
// Tighten AllowOrigins before production deployment.
var corsConfig = middleware.CORSConfig{
	AllowOrigins:     []string{"*"},
	AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodPatch, http.MethodHead},
	AllowHeaders:     []string{"Content-Type", "Authorization", "X-Requested-With", "Accept", "Origin"},
	AllowCredentials: false,
	MaxAge:           86400,
}

func main() {
	e := echo.New()

	// load config (missing file falls back to built-in defaults)
	if err := config.InitConfig("config.toml"); err != nil {
		panic("failed to load config: " + err.Error())
	}

	// Constitution VI (fail fast): an unreachable database MUST terminate
	// startup with a non-zero exit instead of silently degrading.
	if err := model.InitDB(config.Cfg.Database.MySQL.DSN); err != nil {
		e.Logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}

	// Feature 002: seed system_settings once (config.toml only seeds first run),
	// then let DB-authoritative values override the startup-read settings
	// (restart-effective keys: scheduler concurrency, storage root).
	if err := settings.Seed(); err != nil {
		e.Logger.Error("failed to seed system settings", "error", err)
		os.Exit(1)
	}
	if v, ok := settings.GetInt(settings.KeySchedulerConcurrency); ok {
		config.Cfg.Crawler.Concurrency = v
	}
	if v, ok := settings.GetString(settings.KeyStorageRoot); ok {
		config.Cfg.Crawler.StorageRoot = v
	}

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(corsConfig))
	e.Use(middleware.RequestID())
	e.Use(middle.CostTime)

	router.Router(e)

	// crawler scheduler: consumes queued crawl/check runs (concurrency from config).
	crawler.StartScheduler(context.Background())
	defer crawler.StopScheduler()

	if err := e.Start(config.Cfg.Server.Port); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
