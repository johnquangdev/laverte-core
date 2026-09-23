package admin

import (
	"time"

	"github.com/labstack/echo/v4"

	paymentadminuc "github.com/johnquangdev/laverte-core/usecase/paymentadmin"
)

func InitPayments(g *echo.Group, uc paymentadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, loc *time.Location) {
	h := newPaymentHandler(uc, handleErr, handleOK, loc)
	payments := g.Group("/payments")
	payments.GET("", h.list)
	payments.POST("/:id/refund", h.refund)

	unmatched := g.Group("/unmatched-transfers")
	unmatched.GET("", h.listUnmatched)
	unmatched.PATCH("/:id/resolve", h.resolveUnmatched)
}
