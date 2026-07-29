package pricing

import (
	"context"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type fakePricingRuleRepo struct{ rules []*model.PricingRule }

func (f *fakePricingRuleRepo) Create(context.Context, *model.PricingRule) error { return nil }
func (f *fakePricingRuleRepo) Supersede(context.Context, uint, *model.PricingRule, time.Time) error {
	return nil
}
func (f *fakePricingRuleRepo) GetByID(context.Context, uint) (*model.PricingRule, error) {
	return nil, nil
}
func (f *fakePricingRuleRepo) ListActiveByCategory(_ context.Context, category string, _ time.Time) ([]*model.PricingRule, error) {
	var out []*model.PricingRule
	for _, r := range f.rules {
		if r.Category == category {
			out = append(out, r)
		}
	}
	return out, nil
}

func int64p(v int64) *int64 { return &v }
func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }

func TestComputeHourlyWithinBaseHours(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(200000), ExtraHourPrice: int64p(50000),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Minute)
	price, err := uc.Compute(context.Background(), model.HomeCategoryHome, model.PricingRuleTypeHourly, start, end, start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if price != 200000 {
		t.Errorf("price = %d, want 200000", price)
	}
}

func TestComputeHourlyChargesRoundedExtraHours(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(200000), ExtraHourPrice: int64p(50000),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(3*time.Hour + 20*time.Minute) // 1h20m extra -> rounds up to 2 extra hours
	price, err := uc.Compute(context.Background(), model.HomeCategoryHome, model.PricingRuleTypeHourly, start, end, start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if price != 200000+2*50000 {
		t.Errorf("price = %d, want %d", price, 200000+2*50000)
	}
}

func TestComputeOvernightWithinWindow(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000), WindowStart: strp("22:00"), WindowEnd: strp("06:00"),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 23, 30, 0, 0, time.UTC) // inside 22:00-06:00 wrap
	end := start.Add(8 * time.Hour)
	price, err := uc.Compute(context.Background(), model.HomeCategoryNest, model.PricingRuleTypeOvernight, start, end, start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if price != 500000 {
		t.Errorf("price = %d, want 500000", price)
	}
}

func TestComputeOvernightOutsideWindowRejected(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000), WindowStart: strp("22:00"), WindowEnd: strp("06:00"),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC) // 14:00 is outside 22:00-06:00
	end := start.Add(8 * time.Hour)
	if _, err := uc.Compute(context.Background(), model.HomeCategoryNest, model.PricingRuleTypeOvernight, start, end, start); err == nil {
		t.Fatal("Compute() outside window = nil error, want error")
	}
}

// The round-up boundary is the single most delicate line in the money path, so
// pin both sides of it: an exact multiple of an hour must NOT buy an extra hour,
// and one second past it must.
func TestComputeHourlyExactHourBoundaries(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(200000), ExtraHourPrice: int64p(50000),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		dur  time.Duration
		want int64
	}{
		{"exactly base hours charges base only", 2 * time.Hour, 200000},
		{"one second over base buys a whole hour", 2*time.Hour + time.Second, 250000},
		{"exactly one extra hour does not buy a second", 3 * time.Hour, 250000},
		{"one second over that buys the second", 3*time.Hour + time.Second, 300000},
		{"exactly three extra hours", 5 * time.Hour, 350000},
	}
	for _, c := range cases {
		got, err := uc.Compute(context.Background(), model.HomeCategoryHome,
			model.PricingRuleTypeHourly, start, start.Add(c.dur), start)
		if err != nil {
			t.Fatalf("%s: Compute() error = %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: price = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestComputeDayFlatPrice(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeDay,
		FlatPrice: int64p(900000),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	got, err := uc.Compute(context.Background(), model.HomeCategoryNest,
		model.PricingRuleTypeDay, start, start.Add(24*time.Hour), start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if got != 900000 {
		t.Errorf("price = %d, want 900000", got)
	}
}

// A day rule carries no window, so it must not be gated on one.
func TestComputeDayIgnoresTimeOfDay(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeDay,
		FlatPrice: int64p(900000),
	}}}
	uc := New(repo)
	for _, hour := range []int{0, 3, 13, 23} {
		start := time.Date(2026, 8, 1, hour, 0, 0, 0, time.UTC)
		if _, err := uc.Compute(context.Background(), model.HomeCategoryNest,
			model.PricingRuleTypeDay, start, start.Add(24*time.Hour), start); err != nil {
			t.Errorf("hour %02d: Compute() error = %v", hour, err)
		}
	}
}

// A malformed window must surface, not silently behave as 00:00 — that would
// widen or narrow the window and change which bookings are accepted.
func TestComputeOvernightRejectsMalformedWindow(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000), WindowStart: strp("2200"), WindowEnd: strp("06:00"),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)

	if _, err := uc.Compute(context.Background(), model.HomeCategoryNest,
		model.PricingRuleTypeOvernight, start, start.Add(8*time.Hour), start); err == nil {
		t.Fatal("Compute() accepted a malformed window_start, want an error")
	}
}

// windowStart == windowEnd would otherwise refuse every booking silently.
func TestComputeOvernightRejectsZeroWidthWindow(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000), WindowStart: strp("22:00"), WindowEnd: strp("22:00"),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 22, 0, 0, 0, time.UTC)

	if _, err := uc.Compute(context.Background(), model.HomeCategoryNest,
		model.PricingRuleTypeOvernight, start, start.Add(8*time.Hour), start); err == nil {
		t.Fatal("Compute() accepted a zero-width window, want an error")
	}
}

func TestComputeNoMatchingRule(t *testing.T) {
	uc := New(&fakePricingRuleRepo{})
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	if _, err := uc.Compute(context.Background(), model.HomeCategoryHome, model.PricingRuleTypeDay, start, start.Add(time.Hour), start); err == nil {
		t.Fatal("Compute() with no rules = nil error, want error")
	}
}
