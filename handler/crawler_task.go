package handler

import (
	"errors"

	"github.com/labstack/echo/v5"
	"sg.scout/entity/crawler"
	"sg.scout/response"
	crawlersvc "sg.scout/service/crawler"
)

// respErr maps service sentinel errors to the unified error envelope.
// HTTP stays 200; business distinction lives in the envelope (code/message),
// matching the project's response convention (response.NotOk family).
func respErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, crawlersvc.ErrBadRequest):
		return response.NotOk(c, err.Error())
	case errors.Is(err, crawlersvc.ErrNotFound):
		return response.NotOk(c, err.Error())
	case errors.Is(err, crawlersvc.ErrConflict):
		return response.NotOk(c, err.Error())
	default:
		return response.NotOk(c, "服务异常："+err.Error())
	}
}

// --- Task endpoints (contracts/api.md) ---

// CreateTask POST /crawler/tasks — archive a task config (status=pending).
func (*_Crawler) CreateTask(c *echo.Context) error {
	var req crawler.CreateTaskReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	t, err := crawlersvc.CreateTask(&req)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, t)
}

// ListTasks GET /crawler/tasks — task list, optional ?status= filter.
func (*_Crawler) ListTasks(c *echo.Context) error {
	status := c.QueryParam("status")
	if status != "" && status != "pending" && status != "queued" &&
		status != "running" && status != "stopped" && status != "done" {
		return response.NotOk(c, "参数错误：无效的 status")
	}
	list, total, err := crawlersvc.ListTasks(status)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, map[string]any{"list": list, "total": total})
}

// StartTask POST /crawler/tasks/{id}/start — archive a crawl run and enqueue
// it (FR-029: create-then-run split).
func (*_Crawler) StartTask(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	run, err := crawlersvc.CreateRun(id, "crawl")
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, crawlersvc.RunViewOf(run))
}

// GetRun GET /crawler/runs/{id} — run detail + per-page outcome list.
func (*_Crawler) GetRun(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	dv, err := crawlersvc.GetRunDetail(id)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, dv)
}

// Settings GET /crawler/settings — replaced by handler/settings.go (US4).
// _ = (*_Crawler)(nil)

// GetPage GET /crawler/pages/{id} — page detail: content + version timeline.
func (*_Crawler) GetPage(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	dv, err := crawlersvc.GetPageDetail(id)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, dv)
}

// GetTask GET /crawler/tasks/{id} — task detail + latest_run + recent runs.
func (*_Crawler) GetTask(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	tv, runs, err := crawlersvc.GetTask(id)
	if err != nil {
		return respErr(c, err)
	}
	detail := map[string]any{"task": tv, "runs": runs}
	if len(runs) > 0 {
		detail["latest_run"] = runs[0]
	} else {
		detail["latest_run"] = nil
	}
	return response.Ok(c, detail)
}
