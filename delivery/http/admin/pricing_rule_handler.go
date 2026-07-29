package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	pricingadminuc "github.com/johnquangdev/laverte-home/usecase/pricingadmin"
)

type PricingRuleHandler struct {
	uc        pricingadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newPricingRuleHandler(uc pricingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *PricingRuleHandler {
	return &PricingRuleHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *PricingRuleHandler) create(c echo.Context) error {
	var req payload.UpsertPricingRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Create(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *PricingRuleHandler) supersede(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.UpsertPricingRuleRequest
	if err = c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Supersede(c.Request().Context(), uint(id), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *PricingRuleHandler) list(c echo.Context) error {
	category := c.QueryParam("category")
	resp, err := h.uc.ListByCategory(c.Request().Context(), category)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
