package webhook

import (
	"github.com/labstack/echo/v4"

	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
)

// Init mounts the provider callback. It carries no JWT — the request is
// authenticated by the signature VerifyWebhook checks.
func Init(g *echo.Group, uc billinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.POST("/sepay", h.sepay)
}
