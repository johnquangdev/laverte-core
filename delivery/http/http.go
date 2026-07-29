package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

type Server struct {
	echo *echo.Echo
	cfg  config.Config
	log  *zap.Logger
}

// NewServer builds the Echo instance with base middleware and /health only.
// Task 6 rewrites it to take a Deps struct once there are route groups to mount.
func NewServer(cfg config.Config, log *zap.Logger) *Server {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
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
