package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/util/ratelimit"
)

func TestRateLimitByIPDeniesOverLimit(t *testing.T) {
	limiter := ratelimit.NewMemory()
	mw := RateLimitByIP("test", limiter, 1, time.Minute)
	handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1111"

	rec1 := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec1)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec2)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second call status = %d, want 429", rec2.Code)
	}
}

func TestRateLimitByPhoneFallsBackToIP(t *testing.T) {
	limiter := ratelimit.NewMemory()
	mw := RateLimitByPhone("test", limiter, 1, time.Minute)
	handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "5.6.7.8:2222"
	rec := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// errLimiter always fails, standing in for Redis being unreachable.
type errLimiter struct{}

func (errLimiter) Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error) {
	return ratelimit.Result{}, errors.New("redis down")
}

// The limiter fails OPEN on purpose: an infra blip should degrade protection,
// not take the API offline. Note this is the opposite of JWTAuth's blacklist
// lookup, which fails closed — see that middleware's comment for why.
func TestRateLimitAllowsWhenLimiterErrors(t *testing.T) {
	mw := RateLimitByIP("test", errLimiter{}, 1, time.Minute)
	reached := false
	handler := mw(func(c echo.Context) error {
		reached = true
		return c.NoContent(http.StatusOK)
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "9.9.9.9:3333"
	rec := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — a limiter error must not block the request", rec.Code)
	}
	if !reached {
		t.Error("handler never ran, so the limiter failed closed instead of open")
	}
}
