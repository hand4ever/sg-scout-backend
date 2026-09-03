package handler

import (
	"github.com/labstack/echo/v5"
	"sg.scout/entity/settings"
	"sg.scout/response"
	settingsvc "sg.scout/service/settings"
)

// SettingsHandler serves the system-settings API (feature 002 US4,
// contracts/api.md): DB-authoritative runtime config with audit trail.
var SettingsHandler = &_SettingsHandler{}

type _SettingsHandler struct{}

// Get GET /crawler/settings — every registered key with current/default value
// and effect timing (no secrets are ever exposed, FR-012).
func (*_SettingsHandler) Get(c *echo.Context) error {
	items, err := settingsvc.Items()
	if err != nil {
		return response.NotOk(c, "读取配置失败："+err.Error())
	}
	return response.Ok(c, map[string]any{"items": items})
}

// Update PUT /crawler/settings — partial update with validation + audit note.
func (*_SettingsHandler) Update(c *echo.Context) error {
	var req settings.UpdateSettingsReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	if len(req.Items) == 0 {
		return response.NotOk(c, "参数错误：items 不能为空")
	}
	m := make(map[string]any, len(req.Items))
	for _, it := range req.Items {
		m[it.Key] = it.Value
	}
	restartKeys, err := settingsvc.UpdateItems(m, req.Note)
	if err != nil {
		return response.NotOk(c, err.Error())
	}
	items, err := settingsvc.Items()
	if err != nil {
		return response.NotOk(c, "读取配置失败："+err.Error())
	}
	return response.Ok(c, map[string]any{"items": items, "restart_effective": restartKeys})
}

// Reset POST /crawler/settings/reset — restore key (or all) to defaults.
func (*_SettingsHandler) Reset(c *echo.Context) error {
	var req settings.ResetSettingsReq
	if err := c.Bind(&req); err != nil {
		return response.NotOk(c, "参数有误")
	}
	restartKeys, err := settingsvc.Reset(req.Key, req.Note)
	if err != nil {
		return response.NotOk(c, err.Error())
	}
	items, err := settingsvc.Items()
	if err != nil {
		return response.NotOk(c, "读取配置失败："+err.Error())
	}
	return response.Ok(c, map[string]any{"items": items, "restart_effective": restartKeys})
}

// History GET /crawler/settings/history — audit trail, optional ?key= filter.
func (*_SettingsHandler) History(c *echo.Context) error {
	rows, err := settingsvc.History(c.QueryParam("key"), 0)
	if err != nil {
		return response.NotOk(c, "读取历史失败："+err.Error())
	}
	return response.Ok(c, map[string]any{"list": rows})
}
