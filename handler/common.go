package handler

import (
	"runtime"
	"time"

	"github.com/labstack/echo/v5"
	"sg.scout/config"
	"sg.scout/entity/common"
	"sg.scout/response"
)

// Common is the handler instance for public/common endpoints.
var Common = &_Common{}

type _Common struct{}

// Version returns application version information from config.
func (*_Common) Version(c *echo.Context) error {
	cfg := config.Cfg
	return response.Ok(c, common.VersionResponse{
		Version:   cfg.App.Version,
		BuildTime: cfg.App.BuildTime,
		GoVersion: runtime.Version(),
	})
}

// Setting returns key application settings from config.
func (*_Common) Setting(c *echo.Context) error {
	cfg := config.Cfg
	items := []common.SettingItem{
		{Key: "app_name", Value: cfg.App.Name, Description: "Application name"},
		{Key: "app_version", Value: cfg.App.Version, Description: "Application version"},
		{Key: "server_port", Value: cfg.Server.Port, Description: "Server listen port"},
		{Key: "mysql_dsn", Value: cfg.Database.MySQL.DSN, Description: "MySQL connection string"},
	}
	return response.Ok(c, items)
}

// Health returns the liveness probe result.
func (*_Common) Health(c *echo.Context) error {
	return response.Ok(c, common.HealthResponse{
		Status:     "ok",
		ServerTime: time.Now().Format(time.RFC3339),
	})
}
