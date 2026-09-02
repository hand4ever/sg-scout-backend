package response

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

// getCost reads the start time set by middle.CostTime and returns the elapsed time.
func getCost(c *echo.Context) string {
	start, ok := c.Get("i_start_time").(time.Time)
	if !ok {
		return "-"
	}
	return time.Since(start).String()
}

// Ok returns a success response with code 0.
func Ok(c *echo.Context, data any) error {
	respData := &ErrMsg{
		Code:    ErrCodeOk,
		Message: "",
		Data:    data,
	}
	respData.TraceID = c.Response().Header().Get(echo.HeaderXRequestID)
	respData.Cost = getCost(c)
	return c.JSON(http.StatusOK, respData)
}

// NotOk returns a generic business error response (HTTP status stays 200).
func NotOk(c *echo.Context, message string) error {
	respData := &ErrMsg{
		Code:    ErrCodeCustom,
		Message: message,
		Data:    "",
	}
	respData.TraceID = c.Response().Header().Get(echo.HeaderXRequestID)
	respData.Cost = getCost(c)
	return c.JSON(http.StatusOK, respData)
}

// NotOkWithCode returns an error response with a specific code.
func NotOkWithCode(c *echo.Context, message string, code Code) error {
	respData := &ErrMsg{
		Code:    code,
		Message: message,
		Data:    "",
	}
	respData.TraceID = c.Response().Header().Get(echo.HeaderXRequestID)
	respData.Cost = getCost(c)
	return c.JSON(http.StatusOK, respData)
}

// NotOkWithData returns an error response that also carries a data payload.
func NotOkWithData(c *echo.Context, message string, code Code, data any) error {
	respData := &ErrMsg{
		Code:    code,
		Message: message,
		Data:    data,
	}
	respData.TraceID = c.Response().Header().Get(echo.HeaderXRequestID)
	respData.Cost = getCost(c)
	return c.JSON(http.StatusOK, respData)
}
