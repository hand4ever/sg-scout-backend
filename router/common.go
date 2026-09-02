package router

import (
	"github.com/labstack/echo/v5"
	"sg.scout/handler"
)

func common(e *echo.Echo) {
	c := e.Group("/common")
	c.GET("/version", handler.Common.Version)
	c.GET("/health", handler.Common.Health)
	c.GET("/setting", handler.Common.Setting)
}
