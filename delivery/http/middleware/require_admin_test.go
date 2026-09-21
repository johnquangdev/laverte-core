package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/util"
)

func withClaims(c echo.Context, userID uint) {
	c.Set(string(ClaimsKey), &util.Claims{UserID: userID})
}

func TestRequireAdminAllowsSuperadminFromEnv(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 99)

	handler := RequireAdmin(config.Config{AdminUserIDs: []uint{99}}, nil)(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRequireAdminRejectsPlainUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 1)

	resolver := AdminRoleResolverFunc(func(_ context.Context, _ uint) (string, error) { return model.RoleUser, nil })
	handler := RequireAdmin(config.Config{}, resolver)(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRequireAdminAllowsGrantedAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 5)

	resolver := AdminRoleResolverFunc(func(context.Context, uint) (string, error) { return model.RoleAdmin, nil })
	handler := RequireAdmin(config.Config{}, resolver)(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a CMS-granted admin", rec.Code)
	}
}

// Entitlement that cannot be confirmed must not be granted.
func TestRequireAdminRejectsOnResolverError(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 5)

	resolver := AdminRoleResolverFunc(func(context.Context, uint) (string, error) {
		return "", errors.New("db down")
	})
	handler := RequireAdmin(config.Config{}, resolver)(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 when the resolver fails", rec.Code)
	}
}

func TestRequireSuperAdminAllowsEnvListedUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 42)

	handler := RequireSuperAdmin(config.Config{AdminUserIDs: []uint{42}})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// The tier that stops an admin widening the admin set — including to itself.
// A granted admin must NOT satisfy RequireSuperAdmin.
func TestRequireSuperAdminRejectsGrantedAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 5)

	called := false
	handler := RequireSuperAdmin(config.Config{AdminUserIDs: []uint{42}})(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if called {
		t.Error("a non-superadmin reached a superadmin-only handler")
	}
}

func TestRequireSuperAdminRejectsUnauthenticated(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := RequireSuperAdmin(config.Config{AdminUserIDs: []uint{42}})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
