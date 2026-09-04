package handler

import (
	"github.com/labstack/echo/v5"
	"sg.scout/response"
	proofreadsvc "sg.scout/service/proofread"
)

// ProofreadRun is the handler instance for auto-check run endpoints
// (contracts/api.md feature 005 §6-8). Thin layer: param -> service -> response.
var ProofreadRun = &_ProofreadRun{}

type _ProofreadRun struct{}

// StartAutoCheck POST /proofreads/{id}/auto-check — kick off an async
// auto-proofreading run over all enabled engines (contracts §6).
func (*_ProofreadRun) StartAutoCheck(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	run, err := proofreadsvc.StartAutoCheck(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, map[string]any{"run": run})
}

// ListRuns GET /proofreads/{id}/runs — run list, newest first (contracts §7).
func (*_ProofreadRun) ListRuns(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	list, err := proofreadsvc.ListRuns(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, map[string]any{"list": list})
}

// RunDetail GET /proofreads/{id}/runs/{rid} — one run with engine-level
// results (polling target, contracts §8).
func (*_ProofreadRun) RunDetail(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	rid, err := pathID(c, "rid")
	if err != nil {
		return err
	}
	run, err := proofreadsvc.RunDetail(id, rid)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, map[string]any{"run": run})
}
