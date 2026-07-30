package payload

import "time"

type CreateWalkInBookingRequest struct {
	HomeID        uint      `json:"home_id" validate:"required"`
	CustomerName  string    `json:"customer_name" validate:"required,max=100"`
	CustomerPhone string    `json:"customer_phone" validate:"required,max=20"`
	StartTime     time.Time `json:"start_time" validate:"required"`
	EndTime       time.Time `json:"end_time" validate:"required"`
	BookingType   string    `json:"booking_type" validate:"required"`
	PaidCash      bool      `json:"paid_cash"`
}

type SetLockCodeRequest struct {
	Code string `json:"code" validate:"required"`
}
