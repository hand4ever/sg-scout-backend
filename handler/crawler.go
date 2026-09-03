package handler

import (
	"strconv"

	"github.com/labstack/echo/v5"
	"sg.scout/response"
)

// Crawler is the handler instance for /crawler endpoints (contracts/api.md).
// Thin layer: binds params -> service call -> response wrapper. Methods are
// wired progressively per user story; unimplemented ones return an explicit
// "not implemented" error (never silent).
var Crawler = &_Crawler{}

type _Crawler struct{}

// pathID parses a uint64 path parameter (:id), responding 400 on bad input.
func pathID(c *echo.Context, name string) (uint64, error) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, response.NotOk(c, "参数错误：无效的 "+name)
	}
	return id, nil
}

func notImplemented(c *echo.Context) error {
	return response.NotOk(c, "该接口尚未实现（开发中）")
}

// --- Task endpoints (CreateTask/ListTasks/GetTask live in crawler_task.go) ---

func (*_Crawler) CheckTask(c *echo.Context) error   { return notImplemented(c) }
func (*_Crawler) StopTask(c *echo.Context) error    { return notImplemented(c) }
func (*_Crawler) DeleteTask(c *echo.Context) error  { return notImplemented(c) }

// --- Run endpoints ---

func (*_Crawler) RetryFailed(c *echo.Context) error    { return notImplemented(c) }

// --- Page & version endpoints ---

func (*_Crawler) GetPageVersion(c *echo.Context) error      { return notImplemented(c) }
func (*_Crawler) DeletePage(c *echo.Context) error          { return notImplemented(c) }
