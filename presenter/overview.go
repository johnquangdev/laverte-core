package presenter

import "time"

// OverviewResponse reports two figures on deliberately different clocks, and they do
// not divide into each other: TotalRevenueVND is cash-basis — payments whose paid_at
// falls in the range — while BookingCount is occupancy-basis — stays whose start_time
// does. A stay paid in July for an August night is money in July and occupancy in
// August, so treating the pair as an average price per booking is wrong.
type OverviewResponse struct {
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	TotalRevenueVND int64     `json:"total_revenue_vnd"`
	BookingCount    int64     `json:"booking_count"`
}

// OverviewBreakdownResponse details one month for the admin overview, plus a
// trailing series long enough to compare it with the same month a year earlier.
// Revenue and booking counts follow OverviewResponse's two clocks: revenue by
// paid_at, bookings by start_time.
type OverviewBreakdownResponse struct {
	Month string `json:"month"` // YYYY-MM
	// Months is oldest first and always complete: a month with no activity is
	// present with zeros, so a chart never has to guess at a gap.
	Months []OverviewMonth `json:"months"`
	// OccupancyPercent is hours with a confirmed or completed stay divided by the
	// hours the counted homes could have been sold in the month.
	OccupancyPercent float64 `json:"occupancy_percent"`
	// BookedDays are the days of the month, in the business zone, touched by at
	// least one confirmed or completed stay.
	BookedDays []int          `json:"booked_days"`
	Homes      []OverviewHome `json:"homes"`
}

type OverviewMonth struct {
	Month          string `json:"month"`
	RevenueVND     int64  `json:"revenue_vnd"`
	BookingCount   int64  `json:"booking_count"`
	HourlyCount    int64  `json:"hourly_count"`
	OvernightCount int64  `json:"overnight_count"`
	DayCount       int64  `json:"day_count"`
}

// OverviewHome is one home's share of the month. OccupiedHours is for display
// only; money never passes through a float here.
type OverviewHome struct {
	HomeID           uint    `json:"home_id"`
	Name             string  `json:"name"`
	Category         string  `json:"category"`
	IsActive         bool    `json:"is_active"`
	BookingCount     int64   `json:"booking_count"`
	OccupiedHours    float64 `json:"occupied_hours"`
	OccupancyPercent float64 `json:"occupancy_percent"`
	RevenueVND       int64   `json:"revenue_vnd"`
}
