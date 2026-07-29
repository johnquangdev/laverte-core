package webhook

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        billinguc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc billinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) sepay(c echo.Context) error {
	// The HMAC covers the exact bytes the provider sent — binding into a struct
	// and re-marshaling changes key order and spacing, invalidating it.
	raw, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot read body"})
	}
	if err := h.uc.HandleSePayWebhook(c.Request().Context(), raw, c.Request().Header); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
