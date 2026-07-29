package presenter

import "github.com/johnquangdev/laverte-home/model"

type PricingRuleResponse struct {
	ID             uint    `json:"id"`
	Category       string  `json:"category"`
	RuleType       string  `json:"rule_type"`
	BaseHours      *int    `json:"base_hours"`
	BasePrice      *int64  `json:"base_price"`
	ExtraHourPrice *int64  `json:"extra_hour_price"`
	WindowStart    *string `json:"window_start"`
	WindowEnd      *string `json:"window_end"`
	FlatPrice      *int64  `json:"flat_price"`
	IsActive       bool    `json:"is_active"`
}

func ToPricingRuleResponse(r *model.PricingRule) PricingRuleResponse {
	return PricingRuleResponse{
		ID: r.ID, Category: r.Category, RuleType: r.RuleType, BaseHours: r.BaseHours,
		BasePrice: r.BasePrice, ExtraHourPrice: r.ExtraHourPrice, WindowStart: r.WindowStart,
		WindowEnd: r.WindowEnd, FlatPrice: r.FlatPrice, IsActive: r.IsActive,
	}
}
