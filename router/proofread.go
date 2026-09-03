package router

import (
	"github.com/labstack/echo/v5"
	"sg.scout/handler"
)

// proofread registers the proofread module route group
// (/proofreads, contracts/api.md feature 004).
func proofread(e *echo.Echo) {
	g := e.Group("/proofreads")
	// Document endpoints (US1/US5/US6).
	g.GET("", handler.Proofread.ListDocs)
	g.POST("", handler.Proofread.CreateDoc)
	g.GET("/:id", handler.Proofread.GetDoc)
	g.DELETE("/:id", handler.Proofread.DeleteDoc)
	g.POST("/:id/upgrade", handler.Proofread.UpgradeDoc)
	g.POST("/:id/revision-doc", handler.Proofread.DeriveRevisionDoc)
	// Card endpoints (US2).
	g.POST("/:id/cards", handler.Proofread.CreateCard)
	g.PATCH("/:id/cards/:cid", handler.Proofread.UpdateCard)
	g.DELETE("/:id/cards/:cid", handler.Proofread.DeleteCard)
	g.POST("/:id/cards/:cid/state", handler.Proofread.SetCardState)
	// Log & revision/export endpoints (US4/US5).
	g.GET("/:id/logs", handler.Proofread.ListLogs)
	g.GET("/:id/revision", handler.Proofread.RevisionPreview)
	g.GET("/:id/revision/export", handler.Proofread.ExportRevision)
	g.GET("/:id/errata/export", handler.Proofread.ExportErrata)
}
