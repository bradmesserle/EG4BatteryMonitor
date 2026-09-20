package endpoints

import (
	"github.com/eg4/battery/monitor/internal/templates"
	"github.com/labstack/echo/v5"
)

func Home(c *echo.Context) error {

	var cmp = templates.Home()
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return cmp.Render(c.Request().Context(), c.Response())
}
