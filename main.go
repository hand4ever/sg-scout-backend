package main

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"sg.scout/config"
	"sg.scout/middle"
	"sg.scout/model"
	"sg.scout/router"
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

	// MySQL connect failure does NOT block startup: business APIs report their
	// own DB errors, so local front-end integration works without a database.
	if err := model.InitDB(config.Cfg.Database.MySQL.DSN); err != nil {
		e.Logger.Error("failed to connect database", "error", err)
	}

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(corsConfig))
	e.Use(middleware.RequestID())
	e.Use(middle.CostTime)

	router.Router(e)

	if err := e.Start(config.Cfg.Server.Port); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
