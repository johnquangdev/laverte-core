package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"
)

// dateLayout is the only date format admin query params accept.
const dateLayout = "2006-01-02"

type BookingHandler struct {
	uc        bookingadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
	// loc is the business's zone, resolved once at startup. A ?date= is a day in it,
	// not in whatever zone the container happens to run in.
	loc *time.Location
}

func newBookingHandler(uc bookingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, loc *time.Location) *BookingHandler {
	return &BookingHandler{uc: uc, handleErr: handleErr, handleOK: handleOK, loc: loc}
}

func (h *BookingHandler) list(c echo.Context) error {
	homeID, err := strconv.ParseUint(c.QueryParam("home_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid home_id"})
	}

	day := time.Now().In(h.loc)
	if raw := c.QueryParam("date"); raw != "" {
		day, err = time.ParseInLocation(dateLayout, raw, h.loc)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid date, want YYYY-MM-DD"})
		}
	}

	resp, err := h.uc.ListByHomeAndDate(c.Request().Context(), uint(homeID), day)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BookingHandler) create(c echo.Context) error {
	var req payload.CreateWalkInBookingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	adminID := middleware.ClaimsFromContext(c).UserID
	resp, err := h.uc.CreateWalkIn(c.Request().Context(), req, adminID)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BookingHandler) setLockCode(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.SetLockCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	if err := h.uc.SetLockCode(c.Request().Context(), uint(id), req.Code); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}

// idAction adapts the four id-only operations (cancel/complete/no-show/
// send-lock-code) that all answer `{"ok":true}`.
func (h *BookingHandler) idAction(action func(ctx context.Context, id uint) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
		}
		if err := action(c.Request().Context(), uint(id)); err != nil {
			return h.handleErr(c, err)
		}
		return h.handleOK(c, map[string]bool{"ok": true})
	}
}
