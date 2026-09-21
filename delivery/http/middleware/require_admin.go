package middleware

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/model"
)

// AdminRoleResolver reports the admin tier a user currently holds — deliberately
// uncached so a just-revoked admin loses access immediately.
type AdminRoleResolver interface {
	UserRole(ctx context.Context, userID uint) (string, error)
}

type AdminRoleResolverFunc func(ctx context.Context, userID uint) (string, error)

func (f AdminRoleResolverFunc) UserRole(ctx context.Context, userID uint) (string, error) {
	return f(ctx, userID)
}

func RequireAdmin(cfg config.Config, resolver AdminRoleResolver) echo.MiddlewareFunc {
	superadmins := superadminSet(cfg)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := ClaimsFromContext(c)
			if claims == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			if _, ok := superadmins[claims.UserID]; ok {
				return next(c)
			}
			if resolver == nil {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "admin_required"})
			}
			role, err := resolver.UserRole(c.Request().Context(), claims.UserID)
			if err != nil || role != model.RoleAdmin {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "admin_required"})
			}
			return next(c)
		}
	}
}

func RequireSuperAdmin(cfg config.Config) echo.MiddlewareFunc {
	superadmins := superadminSet(cfg)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := ClaimsFromContext(c)
			if claims == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			if _, ok := superadmins[claims.UserID]; !ok {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "superadmin_required"})
			}
			return next(c)
		}
	}
}

func superadminSet(cfg config.Config) map[uint]struct{} {
	set := make(map[uint]struct{}, len(cfg.AdminUserIDs))
	for _, id := range cfg.AdminUserIDs {
		set[id] = struct{}{}
	}
	return set
}
