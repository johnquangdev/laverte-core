package middleware

import (
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
