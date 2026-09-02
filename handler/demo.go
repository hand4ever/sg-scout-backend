package handler

import (
	"github.com/labstack/echo/v5"
	"sg.scout/entity/demo"
	"sg.scout/response"
)

// Demo is the handler instance for /demo endpoints.
var Demo = &_Demo{}

type _Demo struct{}

// Search echoes multi-value query parameters (tags) back, demonstrating
// struct binding and the unified response wrapper.
func (*_Demo) Search(c *echo.Context) error {
	var f demo.Filter
	if err := c.Bind(&f); err != nil {
		return response.NotOk(c, "参数有误")
	}
	return response.Ok(c, f)
}

// Echo returns the path parameter back, demonstrating path binding.
func (*_Demo) Echo(c *echo.Context) error {
	var s demo.EchoReq
	if err := c.Bind(&s); err != nil {
		return response.NotOk(c, "参数有误")
	}
	return response.Ok(c, s)
}
