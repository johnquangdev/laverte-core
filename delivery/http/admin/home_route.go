package admin

import (
	"github.com/labstack/echo/v4"

	homeadminuc "github.com/johnquangdev/laverte-core/usecase/homeadmin"
)

func InitHomes(g *echo.Group, uc homeadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHomeHandler(uc, handleErr, handleOK)
	homes := g.Group("/homes")
	homes.GET("", h.list)
	homes.POST("", h.create)
	homes.PUT("/:id", h.update)
}
