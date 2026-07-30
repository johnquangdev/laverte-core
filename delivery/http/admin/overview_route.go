package admin

import (
	"github.com/labstack/echo/v4"

	overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"
)

func InitOverview(g *echo.Group, uc overviewuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newOverviewHandler(uc, handleErr, handleOK)
	g.GET("/overview", h.summary)
}
