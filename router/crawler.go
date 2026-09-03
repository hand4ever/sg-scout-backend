package router

import (
	"github.com/labstack/echo/v5"
	"sg.scout/handler"
)

// crawler registers the crawler module route group (/crawler, contracts/api.md).
func crawler(e *echo.Echo) {
	g := e.Group("/crawler")
	// Task endpoints (US1/US4).
	g.POST("/tasks", handler.Crawler.CreateTask)
	g.GET("/tasks", handler.Crawler.ListTasks)
	g.GET("/tasks/:id", handler.Crawler.GetTask)
	g.POST("/tasks/:id/start", handler.Crawler.StartTask)
	g.POST("/tasks/:id/check", handler.Crawler.CheckTask)
	g.POST("/tasks/:id/stop", handler.Crawler.StopTask)
	g.DELETE("/tasks/:id", handler.Crawler.DeleteTask)
	g.GET("/tasks/:id/export", handler.Crawler.ExportTask)
	// Run endpoints (US4).
	g.GET("/runs/:id", handler.Crawler.GetRun)
	g.POST("/runs/:id/retry-failed", handler.Crawler.RetryFailed)
	// Page & version endpoints (US1/US3).
	g.GET("/pages/:id", handler.Crawler.GetPage)
	g.GET("/pages/:id/versions/:version", handler.Crawler.GetPageVersion)
	g.DELETE("/pages/:id", handler.Crawler.DeletePage)
	g.GET("/pages/:id/export", handler.Crawler.ExportPage)
	// System settings (read-only).
	g.GET("/settings", handler.Crawler.Settings)
}
