package payload

import "time"

type CreateBookingRequest struct {
	HomeID        uint      `json:"home_id" validate:"required"`
	CustomerName  string    `json:"customer_name" validate:"required,max=100"`
	CustomerPhone string    `json:"customer_phone" validate:"required,max=20"`
	StartTime     time.Time `json:"start_time" validate:"required"`
	EndTime       time.Time `json:"end_time" validate:"required"`
	BookingType   string    `json:"booking_type" validate:"required,oneof=hourly overnight day"`
}
