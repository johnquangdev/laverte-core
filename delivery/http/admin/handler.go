package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        adminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc adminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) listAdmins(c echo.Context) error {
	resp, err := h.uc.ListAdmins(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) grantAdmin(c echo.Context) error {
	var req payload.GrantAdminRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	grantedBy := middleware.ClaimsFromContext(c).UserID
	if err := h.uc.GrantAdmin(c.Request().Context(), req, grantedBy); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}

func (h *Handler) revokeAdmin(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	if err := h.uc.RevokeAdmin(c.Request().Context(), uint(id)); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
