package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/util"
	"github.com/johnquangdev/laverte-core/util/tokenstore"
)

type contextKey string

const ClaimsKey contextKey = "claims"

// JWTAuth authenticates a bearer access token and rejects one that Logout has
// revoked. The store lookup is what makes logout mean anything: the JWT stays
// cryptographically valid until it expires, so without consulting the blacklist
// a logged-out token would keep working for the rest of its TTL.
//
// The blacklist check fails CLOSED — a Redis error rejects the request. This is
// the opposite of the rate limiter's stance on purpose: a limiter that can't
// reach Redis should degrade protection rather than take the API down, but an
// authorization check that can't confirm a token is still valid must not let it
// through.
func JWTAuth(cfg config.Config, store tokenstore.ITokenStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := util.ParseToken(cfg.JWTAccessSecret, tokenStr)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			revoked, err := store.IsBlacklisted(c.Request().Context(), claims.TokenID)
			if err != nil || revoked {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			c.Set(string(ClaimsKey), claims)
			return next(c)
		}
	}
}

func ClaimsFromContext(c echo.Context) *util.Claims {
	v := c.Get(string(ClaimsKey))
	if v == nil {
		return nil
	}
	claims, _ := v.(*util.Claims)
	return claims
}
