package router

import (
	"github.com/labstack/echo/v5"
	"sg.scout/handler"
)

func demo(e *echo.Echo) {
	d := e.Group("/demo")
	d.GET("/search", handler.Demo.Search)
	d.GET("/echo/:str", handler.Demo.Echo)
}
