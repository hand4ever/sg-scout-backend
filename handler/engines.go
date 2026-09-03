package handler

import (
	"github.com/labstack/echo/v5"
	"sg.scout/response"
	"sg.scout/service/crawler/engine"
)

// EnginesHandler serves GET /crawler/engines (feature 002 US1/contracts): the
// engine registry with capabilities and runtime configured/available state.
// It is the data source for the task-create engine picker.
var EnginesHandler = &_EnginesHandler{}

type _EnginesHandler struct{}

// List GET /crawler/engines — engine roster for the frontend picker.
func (*_EnginesHandler) List(c *echo.Context) error {
	return response.Ok(c, map[string]any{"list": engine.List()})
}
