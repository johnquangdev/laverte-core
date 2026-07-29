package model

import "time"

const (
	PricingRuleTypeHourly    = "hourly"
	PricingRuleTypeOvernight = "overnight"
	PricingRuleTypeDay       = "day"
)

// PricingRule defines one price rule for a Home category. WindowStart/End are
// "HH:MM" strings used only by overnight rules to decide whether a booking's
// start time falls inside the overnight window (e.g. "22:00"-"06:00" wraps
// past midnight).
type PricingRule struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Category       string     `gorm:"not null;index" json:"category"`
	RuleType       string     `gorm:"not null" json:"rule_type"`
	BaseHours      *int       `json:"base_hours"`
	BasePrice      *int64     `json:"base_price"`
	ExtraHourPrice *int64     `json:"extra_hour_price"`
	WindowStart    *string    `json:"window_start"`
	WindowEnd      *string    `json:"window_end"`
	FlatPrice      *int64     `json:"flat_price"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveTo    *time.Time `json:"effective_to"`
	IsActive       bool       `gorm:"default:true" json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (PricingRule) TableName() string { return "pricing_rules" }
