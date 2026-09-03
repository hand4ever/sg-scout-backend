package router

import (
	"github.com/labstack/echo/v5"
)

// Router registers all route groups.
func Router(e *echo.Echo) {
	common(e)   // public/common components
	demo(e)     // demo endpoints
	crawler(e)  // crawler module (001-page-crawler-monitor)
}
