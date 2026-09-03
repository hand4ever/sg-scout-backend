package handler

import (
	"strconv"

	"github.com/labstack/echo/v5"
	entityproofread "sg.scout/entity/proofread"
	"sg.scout/response"
	proofreadsvc "sg.scout/service/proofread"
)

// cardPath parses the :cid path parameter (uint64).
func cardPath(c *echo.Context) (uint64, error) {
	return pathID(c, "cid")
}

// CreateCard POST /proofreads/{id}/cards — create a proofreading card.
func (*_Proofread) CreateCard(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var req entityproofread.CardCreateReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	card, err := proofreadsvc.CreateCard(id, &req)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, card)
}

// UpdateCard PATCH /proofreads/{id}/cards/{cid} — edit fields (status untouched).
func (*_Proofread) UpdateCard(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	cid, err := cardPath(c)
	if err != nil {
		return err
	}
	var req entityproofread.CardUpdateReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	card, err := proofreadsvc.UpdateCard(id, cid, &req)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, card)
}

// DeleteCard DELETE /proofreads/{id}/cards/{cid} — remove a card (log kept).
func (*_Proofread) DeleteCard(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	cid, err := cardPath(c)
	if err != nil {
		return err
	}
	if err := proofreadsvc.DeleteCard(id, cid); err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, nil)
}

// SetCardState POST /proofreads/{id}/cards/{cid}/state — re-adjudicate.
func (*_Proofread) SetCardState(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	cid, err := cardPath(c)
	if err != nil {
		return err
	}
	var req entityproofread.CardStateReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	if _, err := strconv.ParseInt(req.Status, 10, 64); err == nil {
		return response.NotOk(c, "参数错误：status 需为 pending/accepted/rejected")
	}
	card, err := proofreadsvc.SetCardState(id, cid, &req)
	if err != nil {
		return respProofreadErr(c, err)
	}
	return response.Ok(c, card)
}
