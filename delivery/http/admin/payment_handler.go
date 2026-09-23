package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/delivery/http/middleware"
	"github.com/johnquangdev/laverte-core/payload"
	paymentadminuc "github.com/johnquangdev/laverte-core/usecase/paymentadmin"
)

// defaultPaymentWindow is how far back the payments list looks without ?from=.
const defaultPaymentWindow = 30

type PaymentHandler struct {
	uc        paymentadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
	loc       *time.Location
}

func newPaymentHandler(uc paymentadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, loc *time.Location) *PaymentHandler {
	return &PaymentHandler{uc: uc, handleErr: handleErr, handleOK: handleOK, loc: loc}
}

// list reads ?from= and ?to= as days in the business zone, with `to` exclusive
// like the overview's, and defaults to the last 30 days including today.
func (h *PaymentHandler) list(c echo.Context) error {
	now := time.Now().In(h.loc)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.loc).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -defaultPaymentWindow)

	if raw := c.QueryParam("from"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, h.loc)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from, want YYYY-MM-DD"})
		}
		from = parsed
	}
	if raw := c.QueryParam("to"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, h.loc)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to, want YYYY-MM-DD"})
		}
		to = parsed
	}

	resp, err := h.uc.List(c.Request().Context(), from, to)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *PaymentHandler) refund(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.NoteRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err = c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	adminID := middleware.ClaimsFromContext(c).UserID
	resp, err := h.uc.Refund(c.Request().Context(), uint(id), adminID, req.Note)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

// listUnmatched shows open transfers unless ?status=all asks for the history.
func (h *PaymentHandler) listUnmatched(c echo.Context) error {
	openOnly := c.QueryParam("status") != "all"
	resp, err := h.uc.ListUnmatched(c.Request().Context(), openOnly)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *PaymentHandler) resolveUnmatched(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.NoteRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err = c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	adminID := middleware.ClaimsFromContext(c).UserID
	resp, err := h.uc.ResolveUnmatched(c.Request().Context(), uint(id), adminID, req.Note)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
