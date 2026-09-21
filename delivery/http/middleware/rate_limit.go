package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/util/ratelimit"
)

func RateLimitByIP(name string, limiter ratelimit.ILimiter, limit int, window time.Duration) echo.MiddlewareFunc {
	return rateLimit(limiter, limit, window, func(c echo.Context) string {
		return "rl:" + name + ":ip:" + c.RealIP()
	})
}

func RateLimitByUser(name string, limiter ratelimit.ILimiter, limit int, window time.Duration) echo.MiddlewareFunc {
	return rateLimit(limiter, limit, window, func(c echo.Context) string {
		if claims := ClaimsFromContext(c); claims != nil {
			return "rl:" + name + ":user:" + strconv.FormatUint(uint64(claims.UserID), 10)
		}
		return "rl:" + name + ":ip:" + c.RealIP()
	})
}

// RateLimitByPhone keys on a form/JSON field named "customer_phone" bound
// earlier in the handler chain via c.Set("customer_phone", phone). Falls back
// to per-IP if the phone hasn't been set yet.
func RateLimitByPhone(name string, limiter ratelimit.ILimiter, limit int, window time.Duration) echo.MiddlewareFunc {
	return rateLimit(limiter, limit, window, func(c echo.Context) string {
		if phone, ok := c.Get("customer_phone").(string); ok && phone != "" {
			return "rl:" + name + ":phone:" + phone
		}
		return "rl:" + name + ":ip:" + c.RealIP()
	})
}

func rateLimit(limiter ratelimit.ILimiter, limit int, window time.Duration, keyFn func(echo.Context) string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			res, err := limiter.Allow(c.Request().Context(), keyFn(c), limit, window)
			if err != nil {
				return next(c) // fail open
			}
			if !res.Allowed {
				retrySec := max(int(math.Ceil(res.RetryAfter.Seconds())), 1)
				c.Response().Header().Set("Retry-After", strconv.Itoa(retrySec))
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			}
			return next(c)
		}
	}
}
