package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
)

type BlockedSlotHandler struct {
	uc        blockedslotuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newBlockedSlotHandler(uc blockedslotuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *BlockedSlotHandler {
	return &BlockedSlotHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *BlockedSlotHandler) list(c echo.Context) error {
	homeID, err := strconv.ParseUint(c.QueryParam("home_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid home_id"})
	}
	resp, err := h.uc.ListByHome(c.Request().Context(), uint(homeID))
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BlockedSlotHandler) create(c echo.Context) error {
	var req payload.CreateBlockedSlotRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	adminID := middleware.ClaimsFromContext(c).UserID
	resp, err := h.uc.Create(c.Request().Context(), req, adminID)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BlockedSlotHandler) remove(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	if err := h.uc.Delete(c.Request().Context(), uint(id)); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
