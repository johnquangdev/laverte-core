package admin

import (
	"time"

	"github.com/labstack/echo/v4"

	overviewuc "github.com/johnquangdev/laverte-core/usecase/overview"
)

func InitOverview(g *echo.Group, uc overviewuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, loc *time.Location) {
	h := newOverviewHandler(uc, handleErr, handleOK, loc)
	g.GET("/overview", h.summary)
	g.GET("/overview/breakdown", h.breakdown)
}
