package handler

import (
	"bytes"
	"fmt"

	"github.com/labstack/echo/v5"
	crawlersvc "sg.scout/service/crawler"
)

// ExportPage GET /crawler/pages/{id}/export — zip of the page artifact dir.
func (*_Crawler) ExportPage(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := crawlersvc.ExportPageZip(&buf, id); err != nil {
		return respErr(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="page-`+itoa(id)+`.zip"`)
	return c.Blob(200, "application/zip", buf.Bytes())
}

// ExportTask GET /crawler/tasks/{id}/export — zip of the whole task dir.
func (*_Crawler) ExportTask(c *echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := crawlersvc.ExportTaskZip(&buf, id); err != nil {
		return respErr(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="task-`+itoa(id)+`.zip"`)
	return c.Blob(200, "application/zip", buf.Bytes())
}

func itoa(id uint64) string {
	return fmt.Sprintf("%d", id)
}
