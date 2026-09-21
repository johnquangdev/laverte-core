package home

import (
	"github.com/labstack/echo/v4"

	homeadminuc "github.com/johnquangdev/laverte-core/usecase/homeadmin"
)

func Init(g *echo.Group, uc homeadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.GET("", h.listActive)
}
