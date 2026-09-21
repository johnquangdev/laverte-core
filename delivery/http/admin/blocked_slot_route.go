package admin

import (
	"github.com/labstack/echo/v4"

	blockedslotuc "github.com/johnquangdev/laverte-core/usecase/blockedslot"
)

func InitBlockedSlots(g *echo.Group, uc blockedslotuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newBlockedSlotHandler(uc, handleErr, handleOK)
	slots := g.Group("/blocked-slots")
	slots.GET("", h.list)
	slots.POST("", h.create)
	slots.DELETE("/:id", h.remove)
}
