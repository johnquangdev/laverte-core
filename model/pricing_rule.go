package model

import (
	"fmt"
	"time"
)

const (
	PricingRuleTypeHourly    = "hourly"
	PricingRuleTypeOvernight = "overnight"
	PricingRuleTypeDay       = "day"
)

func IsValidPricingRuleType(t string) bool {
	return t == PricingRuleTypeHourly || t == PricingRuleTypeOvernight || t == PricingRuleTypeDay
}

// ParseClockMinutes turns an "HH:MM" bound into minutes since midnight. It errors
// rather than defaulting, because a silent 0 reads as 00:00 and silently widens or
// narrows an overnight window — changing which bookings are accepted and priced.
func ParseClockMinutes(hhmm string) (int, error) {
	var h, m int
	if n, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil || n != 2 {
		return 0, fmt.Errorf("gio khong dung dinh dang HH:MM: %q", hhmm)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("gio ngoai khoang hop le: %q", hhmm)
	}
	return h*60 + m, nil
}

// ValidateClockWindow reports whether a window's two bounds are usable. Equal
// bounds are rejected: they would form a zero-width window that silently refuses
// every booking instead of erroring when the rule is created.
func ValidateClockWindow(start, end string) error {
	startMin, err := ParseClockMinutes(start)
	if err != nil {
		return err
	}
	endMin, err := ParseClockMinutes(end)
	if err != nil {
		return err
	}
	if startMin == endMin {
		return fmt.Errorf("khung gio rong: window_start va window_end deu la %q", start)
	}
	return nil
}

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
