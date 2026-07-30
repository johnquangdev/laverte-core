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
