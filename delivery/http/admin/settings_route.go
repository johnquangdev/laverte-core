package admin

import (
	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/presenter"
)

// InitSettings mounts the read-only view of the effective configuration. There
// is no usecase behind it: it is a projection of values fixed at boot.
func InitSettings(g *echo.Group, cfg config.Config, handleOK HandleOKFunc) {
	resp := presenter.ToAdminSettingsResponse(&cfg)
	g.GET("/settings", func(c echo.Context) error { return handleOK(c, resp) })
}
