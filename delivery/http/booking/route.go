package booking

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/payload"
	bookinguc "github.com/johnquangdev/laverte-core/usecase/booking"
)

// bindBookingRequest parses the body and publishes customer_phone into the
// context. Echo runs every middleware before the handler, so the per-phone
// limiter can only see a phone that something earlier in the chain put there;
// the parsed struct rides along so the handler never re-reads the body.
// It takes handleErr as a parameter (rather than closing over a package-level
// name) because Init is the only place that owns one.
func bindBookingRequest(handleErr HandleErrFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var req payload.CreateBookingRequest
			if err := c.Bind(&req); err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			}
			// Validate here, not in the handler: this is the public guest entry point
			// and the per-phone limiter below keys on customer_phone, so an empty or
			// malformed phone must be rejected before it becomes a rate-limit key.
			if err := c.Validate(&req); err != nil {
				return handleErr(c, err)
			}
			c.Set("booking_request", req)
			// Keyed on the canonical form so two spellings of one number ("0900000001"
			// vs "+84900000001") share one rate-limit bucket instead of doubling it.
			c.Set("customer_phone", model.NormalizeVNPhone(req.CustomerPhone))
			return next(c)
		}
	}
}

// Init mounts the public guest-booking route. Route middleware runs in the
// given order: bind first, then the per-phone limiter, then the handler.
func Init(g *echo.Group, uc bookinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, phoneLimit echo.MiddlewareFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.POST("", h.create, bindBookingRequest(handleErr), phoneLimit)
}

// InitAvailability mounts the availability lookup on the public homes group, not
// on the bookings group: it shares the general public rate limit rather than the
// much tighter booking one, so a guest browsing a calendar cannot exhaust the
// quota they need to actually book.
func InitAvailability(g *echo.Group, uc bookinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.GET("/:id/availability", h.availability)
}
