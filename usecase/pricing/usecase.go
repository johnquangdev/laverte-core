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
	// Integer nanoseconds throughout: a float64 hour count would put the rounding
	// decision at an exact-hour boundary on the money path, and whether that is
	// safe depends on how time.Duration.Hours() splits its integer and fractional
	// parts — not something the price a customer pays should rest on.
	duration := end.Sub(start)
	if duration <= 0 {
		return 0, apperr.Validation("end_time phai sau start_time")
	}
	base := time.Duration(*rule.BaseHours) * time.Hour
	if duration <= base {
		return *rule.BasePrice, nil
	}
	extra := duration - base
	extraWhole := int64(extra / time.Hour)
	if extra%time.Hour > 0 {
		extraWhole++ // a partial extra hour is charged as a whole one
	}
	return *rule.BasePrice + extraWhole**rule.ExtraHourPrice, nil
}

func computeFlat(rule *model.PricingRule, start time.Time) (int64, error) {
	if rule.FlatPrice == nil {
		return 0, apperr.Validation("bang gia thieu flat_price")
	}
	if rule.WindowStart != nil && rule.WindowEnd != nil {
		inside, err := withinWindow(start, *rule.WindowStart, *rule.WindowEnd)
		if err != nil {
			return 0, apperr.Internal(err)
		}
		if !inside {
			return 0, apperr.Validation("start_time khong nam trong khung gio ap dung cua rule nay")
		}
	}
	return *rule.FlatPrice, nil
}

// withinWindow reports whether t's clock time falls in [windowStart, windowEnd),
// wrapping past midnight when windowEnd <= windowStart (e.g. 22:00-06:00). The
// interval is half-open: a start exactly at windowStart is inside, one exactly at
// windowEnd is not.
func withinWindow(t time.Time, windowStart, windowEnd string) (bool, error) {
	if err := model.ValidateClockWindow(windowStart, windowEnd); err != nil {
		return false, err
	}
	startMin, err := model.ParseClockMinutes(windowStart)
	if err != nil {
		return false, err
	}
	endMin, err := model.ParseClockMinutes(windowEnd)
	if err != nil {
		return false, err
	}
	minutesOfDay := t.Hour()*60 + t.Minute()
	if startMin < endMin {
		return minutesOfDay >= startMin && minutesOfDay < endMin, nil
	}
	return minutesOfDay >= startMin || minutesOfDay < endMin, nil
}
