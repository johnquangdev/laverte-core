package booking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"

	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	"github.com/johnquangdev/laverte-home/util/ratelimit"
)

// testValidator runs go-playground validation the same way NewServer wires it
// in production, so this test exercises the same `validate:` tags that guard
// the public endpoint, not a permissive stand-in.
type testValidator struct{ v *validator.Validate }

func (tv *testValidator) Validate(i any) error { return tv.v.Struct(i) }

type fakeUC struct{ calls int }

func (f *fakeUC) Create(context.Context, payload.CreateBookingRequest) (*presenter.BookingResponse, error) {
	f.calls++
	return &presenter.BookingResponse{ID: uint(f.calls)}, nil
}

func passErr(c echo.Context, _ error) error { return c.NoContent(http.StatusBadRequest) }
func passOK(c echo.Context, data any) error { return c.JSON(http.StatusOK, data) }

func bookingBody(phone string) string {
	return `{"home_id":1,"customer_name":"Khach","customer_phone":"` + phone + `",` +
		`"start_time":"2026-08-01T10:00:00Z","end_time":"2026-08-01T12:00:00Z","booking_type":"hourly"}`
}

func newRouterWithPhoneLimit(t *testing.T, limit int) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Validator = &testValidator{v: validator.New()}
	limiter := ratelimit.NewMemory()
	phoneLimit := jwtmw.RateLimitByPhone("test", limiter, limit, time.Minute)
	Init(e.Group("/bookings"), &fakeUC{}, passErr, passOK, phoneLimit)
	return e
}

func doBookingRequest(e *echo.Echo, phone, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(bookingBody(phone)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestPhoneLimitSeesPhoneAcrossDifferentIPs proves bindBookingRequest runs
// ahead of the per-phone limiter in the real Echo middleware chain: if the
// ordering in route.go's g.POST(...) were reversed, RateLimitByPhone would
// never see customer_phone (it's set by bindBookingRequest) and would fall
// back to per-IP keying, so two different IPs would never collide and this
// test would fail.
func TestPhoneLimitSeesPhoneAcrossDifferentIPs(t *testing.T) {
	e := newRouterWithPhoneLimit(t, 1)

	rec1 := doBookingRequest(e, "0900000001", "1.1.1.1:1111")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request (ip 1.1.1.1, phone 0900000001) status = %d, want 200, body=%s", rec1.Code, rec1.Body.String())
	}

	rec2 := doBookingRequest(e, "0900000001", "2.2.2.2:2222")
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request (ip 2.2.2.2, SAME phone 0900000001) status = %d, want 429 — the per-phone limiter did not see the phone", rec2.Code)
	}
}

// TestPhoneLimitAllowsDifferentPhonesFromSameIP proves the limiter keys on
// phone, not IP: two different phones from the same IP must both be allowed
// even though a per-phone limit of 1 is in effect.
func TestPhoneLimitAllowsDifferentPhonesFromSameIP(t *testing.T) {
	e := newRouterWithPhoneLimit(t, 1)

	rec1 := doBookingRequest(e, "0900000001", "9.9.9.9:3333")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request (phone 0900000001) status = %d, want 200, body=%s", rec1.Code, rec1.Body.String())
	}

	rec2 := doBookingRequest(e, "0900000002", "9.9.9.9:3333")
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request (SAME ip, different phone 0900000002) status = %d, want 200, body=%s", rec2.Code, rec2.Body.String())
	}
}

// TestPhoneLimitSharesQuotaAcrossPhoneFormats proves bindBookingRequest normalizes the
// phone before publishing it into the context: "0900000001" and "+84900000001" are the
// same Vietnamese number, so they must share one per-phone quota, not double it. If the
// normalization were removed, RateLimitByPhone would see two distinct keys and both
// requests below would be allowed.
func TestPhoneLimitSharesQuotaAcrossPhoneFormats(t *testing.T) {
	e := newRouterWithPhoneLimit(t, 1)

	rec1 := doBookingRequest(e, "0900000001", "3.3.3.3:4444")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request (phone 0900000001) status = %d, want 200, body=%s", rec1.Code, rec1.Body.String())
	}

	rec2 := doBookingRequest(e, "+84900000001", "4.4.4.4:5555")
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request (SAME number written as +84900000001, different IP) status = %d, want 429 — normalization must map both to one quota", rec2.Code)
	}
}
