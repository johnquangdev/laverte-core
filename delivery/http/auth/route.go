package auth

import (
	"github.com/labstack/echo/v4"

	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
)

func Init(g *echo.Group, uc authuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.GET("/google/login-url", h.loginURL)
	g.POST("/google/callback", h.callback)
	g.POST("/password", h.passwordLogin)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}
