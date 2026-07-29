package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
)

type HomeHandler struct {
	uc        homeadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHomeHandler(uc homeadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *HomeHandler {
	return &HomeHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *HomeHandler) create(c echo.Context) error {
	var req payload.CreateHomeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	resp, err := h.uc.Create(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *HomeHandler) update(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.UpdateHomeRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err = c.Validate(&req); err != nil {
		return h.handleErr(c, err)
	}
	resp, err := h.uc.Update(c.Request().Context(), uint(id), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *HomeHandler) list(c echo.Context) error {
	resp, err := h.uc.List(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
