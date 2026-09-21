package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-core/payload"
	authuc "github.com/johnquangdev/laverte-core/usecase/auth"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        authuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc authuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) loginURL(c echo.Context) error {
	resp, err := h.uc.LoginURL(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) callback(c echo.Context) error {
	var req payload.GoogleCallbackRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	resp, err := h.uc.Callback(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) passwordLogin(c echo.Context) error {
	var req payload.PasswordLoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	resp, err := h.uc.PasswordLogin(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) refresh(c echo.Context) error {
	var req payload.RefreshRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	resp, err := h.uc.RefreshToken(c.Request().Context(), req.RefreshToken)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) logout(c echo.Context) error {
	if err := h.uc.Logout(c.Request().Context(), c.Request().Header.Get("Authorization")); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
