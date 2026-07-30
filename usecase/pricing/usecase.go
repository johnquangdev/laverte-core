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
		return computeFlat(rule, start, end)
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

// day is the unit an overnight or day rule's flat price buys. An overnight
// 22:00→06:00 is one unit, not a fraction of one; 22:00 Monday → 06:00 Wednesday
// is two.
const day = 24 * time.Hour

// MaxBookingDuration caps how long a single booking may run. A domain limit rather
// than a deployment knob: a confirmed booking occupies its home for its whole
// range through the overlap exclusion constraint, and even an unpaid hold occupies
// it for the pending TTL, so an unbounded range takes a property off the market
// for the price of one HTTP request.
const MaxBookingDuration = 30 * day

// ValidateDuration rejects a range longer than MaxBookingDuration. Both create
// paths call this rather than testing the constant themselves, so the guest and
// walk-in flows cannot drift apart and the message cannot drift from the limit.
func ValidateDuration(start, end time.Time) error {
	if end.Sub(start) > MaxBookingDuration {
		return apperr.Validation(fmt.Sprintf("thoi gian dat toi da %d ngay", int64(MaxBookingDuration/day)))
	}
	return nil
}

func computeFlat(rule *model.PricingRule, start, end time.Time) (int64, error) {
	if rule.FlatPrice == nil {
		return 0, apperr.Validation("bang gia thieu flat_price")
	}
	// The window applies to start only: an overnight rule's 22:00-06:00 says when the
	// stay may begin, and checking end against it would refuse the 06:00 checkout the
	// rule is named for.
	if rule.WindowStart != nil && rule.WindowEnd != nil {
		inside, err := withinWindow(start, *rule.WindowStart, *rule.WindowEnd)
		if err != nil {
			return 0, apperr.Internal(err)
		}
		if !inside {
			return 0, apperr.Validation("start_time khong nam trong khung gio ap dung cua rule nay")
		}
	}

	// Same integer-only rule as computeHourly above, and for the same reason. Without
	// the unit count a year-long "day" booking is priced at one day's rate and then,
	// once paid, blocks the property for the year.
	duration := end.Sub(start)
	if duration <= 0 {
		return 0, apperr.Validation("end_time phai sau start_time")
	}
	units := int64(duration / day)
	if duration%day > 0 {
		units++ // a partial day is charged as a whole one
	}
	return units * *rule.FlatPrice, nil
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
