package http

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	adminhttp "github.com/johnquangdev/laverte-home/delivery/http/admin"
	authhttp "github.com/johnquangdev/laverte-home/delivery/http/auth"
	bookinghttp "github.com/johnquangdev/laverte-home/delivery/http/booking"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	webhookhttp "github.com/johnquangdev/laverte-home/delivery/http/webhook"
	apperr "github.com/johnquangdev/laverte-home/errors"
	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
	pricingadminuc "github.com/johnquangdev/laverte-home/usecase/pricingadmin"
	"github.com/johnquangdev/laverte-home/util/ratelimit"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

type errBody struct {
	Code    string `json:"code"`
	CodeID  string `json:"code_id"`
	Message string `json:"message"`
}

type Server struct {
	echo *echo.Echo
	cfg  config.Config
	log  *zap.Logger
}

// requestValidator runs the `validate:` struct tags on bound payloads. Without a
// validator registered on Echo those tags are inert decoration — the tags existed
// before this did, and every "required" silently passed.
type requestValidator struct{ v *validator.Validate }

// Validate returns an apperr so a bad payload answers 400 through the shared
// handleErr path, instead of leaking go-playground's internal field message.
func (rv *requestValidator) Validate(i any) error {
	if err := rv.v.Struct(i); err != nil {
		var invalid *validator.InvalidValidationError
		if errors.As(err, &invalid) {
			return apperr.Internal(err)
		}
		var fieldErrs validator.ValidationErrors
		if errors.As(err, &fieldErrs) && len(fieldErrs) > 0 {
			f := fieldErrs[0]
			return apperr.Validation(fmt.Sprintf("truong %q khong hop le (%s)", f.Field(), f.Tag()))
		}
		return apperr.Validation("du lieu gui len khong hop le")
	}
	return nil
}

// Deps carries what the router needs to mount its route groups. It is a struct
// with named fields, not a positional parameter list, because nearly every
// later task adds one more dependency here — a named field can be appended
// without silently shifting the meaning of every existing call site.
type Deps struct {
	Limiter ratelimit.ILimiter
	// TokenStore is read by JWTAuth to reject tokens Logout has revoked.
	TokenStore        tokenstore.ITokenStore
	AuthUC            authuc.IUseCase
	AdminUC           adminuc.IUseCase
	AdminRoleResolver jwtmw.AdminRoleResolver
	HomeAdminUC       homeadminuc.IUseCase
	PricingAdminUC    pricingadminuc.IUseCase
	BlockedSlotUC     blockedslotuc.IUseCase
	BookingUC         bookinguc.IUseCase
	BillingUC         billinguc.IUseCase
}

func NewServer(cfg config.Config, log *zap.Logger, deps Deps) *Server {
	e := echo.New()
	e.HideBanner = true
	e.Validator = &requestValidator{v: validator.New()}
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	// Guards the public, unauthenticated bookings endpoint (and every other route)
	// against an oversized body tying up a request goroutine before validation ever runs.
	e.Use(middleware.BodyLimit("64K"))
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection: "1; mode=block", ContentTypeNosniff: "nosniff",
		XFrameOptions: "DENY", ReferrerPolicy: "no-referrer",
	}))
	allowOrigins := splitOrigins(cfg.CORSOrigins)
	if len(allowOrigins) == 0 {
		allowOrigins = []string{cfg.FrontendURL}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{echo.GET, echo.HEAD, echo.POST, echo.PUT, echo.PATCH, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	handleErr := func(c echo.Context, err error) error {
		if e, ok := apperr.As(err); ok {
			log.Error(e.Error())
			return c.JSON(e.HTTPCode, errBody{Code: string(e.Code), CodeID: e.CodeID, Message: e.Message})
		}
		log.Error(err.Error())
		ie := apperr.Internal(err)
		return c.JSON(ie.HTTPCode, errBody{Code: string(ie.Code), CodeID: ie.CodeID, Message: ie.Message})
	}
	handleOK := func(c echo.Context, data any) error { return c.JSON(http.StatusOK, data) }

	api := e.Group("/api/v1")
	window := time.Minute
	ipLimit := jwtmw.RateLimitByIP("public", deps.Limiter, cfg.RateLimitPublicPerMin, window)
	userLimit := jwtmw.RateLimitByUser("authed", deps.Limiter, cfg.RateLimitAuthedPerMin, window)

	authhttp.Init(api.Group("/auth", ipLimit), deps.AuthUC, handleErr, handleOK)

	// Guests book without an account: this group is deliberately outside the
	// JWT group, protected only by the two rate limiters.
	bookingIPLimit := jwtmw.RateLimitByIP("booking", deps.Limiter, cfg.RateLimitBookingPerMinIP, window)
	bookingPhoneLimit := jwtmw.RateLimitByPhone("booking", deps.Limiter, cfg.RateLimitBookingPerMinPhone, window)
	bookinghttp.Init(api.Group("/bookings", bookingIPLimit), deps.BookingUC, handleErr, handleOK, bookingPhoneLimit)

	// The provider's own retries are the load here, so this limit is much
	// higher than the guest-facing ones.
	webhookLimit := jwtmw.RateLimitByIP("payment-webhook", deps.Limiter, cfg.RateLimitWebhookPerMin, window)
	webhookhttp.Init(api.Group("/webhooks", webhookLimit), deps.BillingUC, handleErr, handleOK)

	authed := api.Group("", jwtmw.JWTAuth(cfg, deps.TokenStore), userLimit)
	requireAdmin := jwtmw.RequireAdmin(cfg, deps.AdminRoleResolver)
	adminGroup := authed.Group("/admin", requireAdmin)
	adminhttp.Init(adminGroup, deps.AdminUC, handleErr, handleOK, jwtmw.RequireSuperAdmin(cfg))
	adminhttp.InitHomes(adminGroup, deps.HomeAdminUC, handleErr, handleOK)
	adminhttp.InitPricingRules(adminGroup, deps.PricingAdminUC, handleErr, handleOK)
	adminhttp.InitBlockedSlots(adminGroup, deps.BlockedSlotUC, handleErr, handleOK)

	return &Server{echo: e, cfg: cfg, log: log}
}

func (s *Server) Echo() *echo.Echo { return s.echo }

func (s *Server) Start() error {
	return s.echo.Start(":" + s.cfg.Port)
}

func splitOrigins(raw string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			if v := trimSpace(raw[start:i]); v != "" {
				out = append(out, v)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
