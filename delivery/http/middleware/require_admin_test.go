package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/util"
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
