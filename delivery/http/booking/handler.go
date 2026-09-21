package booking

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/payload"
	bookinguc "github.com/johnquangdev/laverte-core/usecase/booking"
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

// availability answers the public "when is this home free" question. Times are
// RFC3339 with an explicit offset, like the booking body they will be pasted into
// — deliberately not the admin routes' `YYYY-MM-DD` in the business zone, which
// would need this public handler to carry a *time.Location too.
func (h *Handler) availability(c echo.Context) error {
	homeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	from := time.Now()
	if raw := c.QueryParam("from"); raw != "" {
		from, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from, want RFC3339"})
		}
	}
	to := from.Add(bookinguc.DefaultAvailabilityWindow)
	if raw := c.QueryParam("to"); raw != "" {
		to, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to, want RFC3339"})
		}
	}

	resp, err := h.uc.Availability(c.Request().Context(), uint(homeID), from, to)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
