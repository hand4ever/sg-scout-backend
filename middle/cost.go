package middle

import (
	"time"

	"github.com/labstack/echo/v5"
)

// CostTime records the request start time so response.getCost can report it.
func CostTime(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		start := time.Now()
		c.Set("i_start_time", start)
		err := next(c)
		c.Logger().Info("<CostTime>", "cost", time.Since(start).String())
		return err
	}
}
