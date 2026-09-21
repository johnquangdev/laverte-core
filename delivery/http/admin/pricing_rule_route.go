package admin

import (
	"github.com/labstack/echo/v4"

	pricingadminuc "github.com/johnquangdev/laverte-core/usecase/pricingadmin"
)

func InitPricingRules(g *echo.Group, uc pricingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newPricingRuleHandler(uc, handleErr, handleOK)
	rules := g.Group("/pricing-rules")
	rules.GET("", h.list)
	rules.POST("", h.create)
	rules.PUT("/:id", h.supersede)
}
