package home

import (
	"github.com/labstack/echo/v4"

	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        homeadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc homeadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) listActive(c echo.Context) error {
	resp, err := h.uc.ListActive(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
