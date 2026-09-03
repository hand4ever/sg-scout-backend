package handler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/labstack/echo/v5"
	entityproofread "sg.scout/entity/proofread"
	"sg.scout/response"
	proofreadsvc "sg.scout/service/proofread"
)

// Proofread is the handler instance for /proofreads endpoints
// (contracts/api.md feature 004). Thin layer: bind -> service -> response.
var Proofread = &_Proofread{}

type _Proofread struct{}

// respProofreadErr maps proofread service sentinels to the unified envelope
// (HTTP stays 200; business distinction lives in code/message).
func respProofreadErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, proofreadsvc.ErrBadRequest):
		return response.NotOk(c, err.Error())
	case errors.Is(err, proofreadsvc.ErrNotFound):
		return response.NotOk(c, err.Error())
	case errors.Is(err, proofreadsvc.ErrConflict):
		return response.NotOk(c, err.Error())
	default:
		return response.NotOk(c, "服务异常："+err.Error())
	}
}

// CreateDoc POST /proofreads — create page-bound (1:1) or pasted-text doc.
func (*_Proofread) CreateDoc(c *echo.Context) error {
	var req entityproofread.DocCreateReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	doc, err := proofreadsvc.CreateDoc(&req)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, doc)
}

// ListDocs GET /proofreads — document list with optional source/page filters.
func (*_Proofread) ListDocs(c *echo.Context) error {
	source := c.QueryParam("source")
	if source != "" && source != "all" && source != entityproofread.SourcePage &&
		source != entityproofread.SourceText && source != entityproofread.SourceRevision {
		return response.NotOk(c, "参数错误：无效的 source")
	}
	var pageID uint64
	if raw := c.QueryParam("page_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return response.NotOk(c, "参数错误：无效的 page_id")
		}
		pageID = id
	}
	list, err := proofreadsvc.ListDocs(source, pageID)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, map[string]any{"list": list})
}

// GetDoc GET /proofreads/{id} — detail with draft content, cards, chain.
func (*_Proofread) GetDoc(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	dv, err := proofreadsvc.GetDocDetail(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, dv)
}

// DeleteDoc DELETE /proofreads/{id} — cascade delete doc + cards + logs.
func (*_Proofread) DeleteDoc(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	if err := proofreadsvc.DeleteDoc(id); err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, nil)
}

// UpgradeDoc POST /proofreads/{id}/upgrade — rebind draft to newest version.
func (*_Proofread) UpgradeDoc(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	res, err := proofreadsvc.UpgradeDoc(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, res)
}

// ListLogs GET /proofreads/{id}/logs — read-only audit trail (FR-013).
func (*_Proofread) ListLogs(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	list, err := proofreadsvc.ListLogs(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, map[string]any{"list": list})
}

// RevisionPreview GET /proofreads/{id}/revision — revised text + change marks.
func (*_Proofread) RevisionPreview(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	view, err := proofreadsvc.RevisionPreview(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, view)
}

// ExportRevision GET /proofreads/{id}/revision/export — clean revised file.
func (*_Proofread) ExportRevision(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	text, err := proofreadsvc.RevisionText(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	name := fmt.Sprintf("proofread-%d-revised.md", id)
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.Blob(200, "text/markdown; charset=utf-8", []byte(text))
}

// ExportErrata GET /proofreads/{id}/errata/export — errata CSV (accepted only).
func (*_Proofread) ExportErrata(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	data, err := proofreadsvc.ErrataExport(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	name := fmt.Sprintf("proofread-%d-errata.csv", id)
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.Blob(200, "text/csv; charset=utf-8", data)
}

// DeriveRevisionDoc POST /proofreads/{id}/revision-doc — spawn a child doc
// whose draft is this document's revision (FR-021).
func (*_Proofread) DeriveRevisionDoc(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	doc, err := proofreadsvc.DeriveRevisionDoc(id)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, doc)
}
