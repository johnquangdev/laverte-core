package payload

type UpsertPricingRuleRequest struct {
	Category       string  `json:"category" validate:"required,oneof=home nest"`
	RuleType       string  `json:"rule_type" validate:"required,oneof=hourly overnight day"`
	BaseHours      *int    `json:"base_hours"`
	BasePrice      *int64  `json:"base_price"`
	ExtraHourPrice *int64  `json:"extra_hour_price"`
	WindowStart    *string `json:"window_start"`
	WindowEnd      *string `json:"window_end"`
	FlatPrice      *int64  `json:"flat_price"`
}
