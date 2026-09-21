package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/util"
)

// fakeTokenStore reports whichever revocation state a test needs, so the
// middleware tests never touch Redis.
type fakeTokenStore struct {
	revoked bool
	err     error
}

func (f fakeTokenStore) SaveState(context.Context, string) error             { return nil }
func (f fakeTokenStore) ValidateState(context.Context, string) (bool, error) { return true, nil }
func (f fakeTokenStore) BlacklistToken(context.Context, string) error        { return nil }
func (f fakeTokenStore) IsBlacklisted(context.Context, string) (bool, error) {
	return f.revoked, f.err
}

func TestJWTAuthRejectsMissingHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTAuth(config.Config{JWTAccessSecret: "s"}, fakeTokenStore{})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestJWTAuthAcceptsValidToken(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "s"}
	tokenStr, _ := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7}, time.Hour)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var gotUserID uint
	handler := JWTAuth(cfg, fakeTokenStore{})(func(c echo.Context) error {
		gotUserID = ClaimsFromContext(c).UserID
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUserID != 7 {
		t.Errorf("UserID = %d, want 7", gotUserID)
	}
}

// A revoked token is still cryptographically valid, so this is the only thing
// that makes Logout mean anything.
func TestJWTAuthRejectsRevokedToken(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "s"}
	tokenStr, _ := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7, TokenID: "tok-1"}, time.Hour)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	handler := JWTAuth(cfg, fakeTokenStore{revoked: true})(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("next handler ran for a revoked token")
	}
}

// Authorization must fail closed: if the store can't confirm the token is still
// valid, the request does not get through.
func TestJWTAuthRejectsWhenStoreErrors(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "s"}
	tokenStr, _ := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7, TokenID: "tok-1"}, time.Hour)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTAuth(cfg, fakeTokenStore{err: errors.New("redis down")})(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
