package pricing

import (
	"context"
	"fmt"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	pricingrulerepo "github.com/johnquangdev/laverte-home/repository/pricingrule"
)

type UseCase struct{ repo pricingrulerepo.IRepository }

func New(repo pricingrulerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Compute(ctx context.Context, category, bookingType string, start, end, at time.Time) (int64, error) {
	rules, err := uc.repo.ListActiveByCategory(ctx, category, at)
	if err != nil {
		return 0, apperr.Internal(err)
	}

	var rule *model.PricingRule
	for _, r := range rules {
		if r.RuleType == bookingType {
			rule = r
			break
		}
	}
	if rule == nil {
		return 0, apperr.Validation(fmt.Sprintf("khong tim thay bang gia cho hang %q, loai %q", category, bookingType))
	}

	switch bookingType {
	case model.PricingRuleTypeHourly:
		return computeHourly(rule, start, end)
	case model.PricingRuleTypeOvernight, model.PricingRuleTypeDay:
		return computeFlat(rule, start)
	default:
		return 0, apperr.Validation(fmt.Sprintf("booking_type khong hop le: %q", bookingType))
	}
}

func computeHourly(rule *model.PricingRule, start, end time.Time) (int64, error) {
	if rule.BaseHours == nil || rule.BasePrice == nil || rule.ExtraHourPrice == nil {
		return 0, apperr.Validation("bang gia hourly thieu base_hours/base_price/extra_hour_price")
	}
	durationHours := end.Sub(start).Hours()
	if durationHours <= 0 {
		return 0, apperr.Validation("end_time phai sau start_time")
	}
	if durationHours <= float64(*rule.BaseHours) {
		return *rule.BasePrice, nil
	}
	extraHours := durationHours - float64(*rule.BaseHours)
	extraWhole := int64(extraHours)
	if extraHours > float64(extraWhole) {
		extraWhole++ // round any partial extra hour up to a full extra-hour charge
	}
	return *rule.BasePrice + extraWhole**rule.ExtraHourPrice, nil
}

func computeFlat(rule *model.PricingRule, start time.Time) (int64, error) {
	if rule.FlatPrice == nil {
		return 0, apperr.Validation("bang gia thieu flat_price")
	}
	if rule.WindowStart != nil && rule.WindowEnd != nil {
		if !withinWindow(start, *rule.WindowStart, *rule.WindowEnd) {
			return 0, apperr.Validation("start_time khong nam trong khung gio ap dung cua rule nay")
		}
	}
	return *rule.FlatPrice, nil
}

// withinWindow reports whether t's local HH:MM falls in [windowStart,
// windowEnd), wrapping past midnight when windowEnd <= windowStart (e.g.
// 22:00-06:00).
func withinWindow(t time.Time, windowStart, windowEnd string) bool {
	minutesOfDay := t.Hour()*60 + t.Minute()
	startMin := hhmmToMinutes(windowStart)
	endMin := hhmmToMinutes(windowEnd)
	if startMin <= endMin {
		return minutesOfDay >= startMin && minutesOfDay < endMin
	}
	return minutesOfDay >= startMin || minutesOfDay < endMin
}

func hhmmToMinutes(hhmm string) int {
	var h, m int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &h, &m); err != nil {
		return 0
	}
	return h*60 + m
}
