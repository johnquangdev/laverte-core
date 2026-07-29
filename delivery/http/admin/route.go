package admin

import (
	"github.com/labstack/echo/v4"

	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
)

// Init mounts admin-roster routes. superadminOnly must be
// middleware.RequireSuperAdmin — the group itself is already gated by
// RequireAdmin one level up in delivery/http/http.go.
func Init(g *echo.Group, uc adminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, superadminOnly echo.MiddlewareFunc) {
	h := newHandler(uc, handleErr, handleOK)
	admins := g.Group("/admins", superadminOnly)
	admins.GET("", h.listAdmins)
	admins.POST("", h.grantAdmin)
	admins.DELETE("/:id", h.revokeAdmin)
}
