package handler

import (
	"github.com/labstack/echo/v5"
	entityproofread "sg.scout/entity/proofread"
	"sg.scout/response"
	proofreadsvc "sg.scout/service/proofread"
)

// ProofreadEngine is the handler instance for /proofreads/engines endpoints
// (contracts/api.md feature 005 §1-5). Thin layer: bind -> service -> response.
var ProofreadEngine = &_ProofreadEngine{}

type _ProofreadEngine struct{}

// ListTypes GET /proofreads/engines/types — static engine type registry with
// config field schema (drives the settings form, research D1).
func (*_ProofreadEngine) ListTypes(c *echo.Context) error {
	return response.Ok(c, map[string]any{"list": proofreadsvc.ListTypes()})
}

// List GET /proofreads/engines — engine instances (contracts §2).
func (*_ProofreadEngine) List(c *echo.Context) error {
	list, err := proofreadsvc.ListEngines()
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, map[string]any{"list": list})
}

// Create POST /proofreads/engines — create an engine instance (contracts §3).
func (*_ProofreadEngine) Create(c *echo.Context) error {
	var req entityproofread.EngineCreateReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	if req.EngineType == "" || req.Name == "" {
		return response.NotOk(c, "参数错误：engine_type 与 name 必填")
	}
	e, err := proofreadsvc.CreateEngine(&req)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, e)
}

// Update PATCH /proofreads/engines/{eid} — partial update (contracts §4).
func (*_ProofreadEngine) Update(c *echo.Context) error {
	id, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	var req entityproofread.EngineUpdateReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	if req.Name == nil && req.Enabled == nil && req.Config == nil && req.Note == nil {
		return response.NotOk(c, "参数错误：至少提交一项更新")
	}
	e, err := proofreadsvc.UpdateEngine(id, &req)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, e)
}

// Delete DELETE /proofreads/engines/{eid} — remove an instance (contracts §5).
func (*_ProofreadEngine) Delete(c *echo.Context) error {
	id, err := pathID(c, "eid")
	if err != nil {
		return err
	}
	if err := proofreadsvc.DeleteEngine(id); err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, nil)
}
