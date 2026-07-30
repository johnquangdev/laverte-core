package admin

import (
	"github.com/labstack/echo/v4"

	bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"
)

func InitBookings(g *echo.Group, uc bookingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newBookingHandler(uc, handleErr, handleOK)
	bookings := g.Group("/bookings")
	bookings.GET("", h.list)
	bookings.POST("", h.create)
	bookings.PATCH("/:id/cancel", h.idAction(uc.Cancel))
	bookings.PATCH("/:id/complete", h.idAction(uc.Complete))
	bookings.PATCH("/:id/no-show", h.idAction(uc.NoShow))
	bookings.PATCH("/:id/lock-code", h.setLockCode)
	bookings.POST("/:id/send-lock-code", h.idAction(uc.SendLockCode))
}
