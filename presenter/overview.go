package presenter

import "time"

type OverviewResponse struct {
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	TotalRevenueVND int64     `json:"total_revenue_vnd"`
	BookingCount    int64     `json:"booking_count"`
}
