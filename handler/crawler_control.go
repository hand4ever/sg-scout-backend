package handler

import (
	"strconv"

	"github.com/labstack/echo/v5"
	"sg.scout/response"
	crawlersvc "sg.scout/service/crawler"
	proofreadsvc "sg.scout/service/proofread"
)

// --- Task control endpoints (feature 002 US3, engine-neutral) ---
// Replaces the former not-implemented stubs; see contracts/api.md.

// CheckTask POST /crawler/tasks/{id}/check — manual change-check run (FR-007).
func (*_Crawler) CheckTask(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	run, err := crawlersvc.CheckTaskRun(id)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, run)
}

// StopTask POST /crawler/tasks/{id}/stop — cancel queued/running run (FR-026).
func (*_Crawler) StopTask(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	run, err := crawlersvc.StopActiveRun(id)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, run)
}

// DeleteTask DELETE /crawler/tasks/{id} — cascade delete (FR-021).
// Proofread documents bound to the task are removed first (feature 004
// FR-019); the crawler service itself stays proofread-agnostic (no package
// cycle: proofread → crawler is one-directional, research D9).
func (*_Crawler) DeleteTask(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := proofreadsvc.DeleteByTask(id); err != nil {
		return respErr(c, err)
	}
	if err := crawlersvc.DeleteTask(id); err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, nil)
}

// RetryFailed POST /crawler/runs/{id}/retry-failed — re-fetch failed pages.
func (*_Crawler) RetryFailed(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	run, err := crawlersvc.RetryFailedRun(id)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, run)
}

// GetPageVersion GET /crawler/pages/{id}/versions/{version} — version body.
func (*_Crawler) GetPageVersion(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rawVersion := c.Param("version")
	version, err := strconv.Atoi(rawVersion)
	if err != nil || version < 1 {
		return response.NotOk(c, "参数错误：无效的 version")
	}
	v, err := crawlersvc.GetPageVersionContent(id, version)
	if err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, v)
}

// DeletePage DELETE /crawler/pages/{id} — delete page + versions + files.
// Page-bound proofread documents are removed first (feature 004 FR-019,
// research D9; DeleteByPage is idempotent).
func (*_Crawler) DeletePage(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := proofreadsvc.DeleteByPage(id); err != nil {
		return respErr(c, err)
	}
	if err := crawlersvc.DeletePage(id); err != nil {
		return respErr(c, err)
	}
	return response.Ok(c, nil)
}
