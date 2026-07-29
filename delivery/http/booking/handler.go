package booking

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        bookinguc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc bookinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) create(c echo.Context) error {
	// bindBookingRequest already consumed the body; re-binding here would read
	// an empty reader.
	req, ok := c.Get("booking_request").(payload.CreateBookingRequest)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Create(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
