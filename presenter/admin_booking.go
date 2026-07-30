package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

// AdminBookingResponse carries the same fields as BookingResponse plus the
// operational ones a guest must never see: the door code, the calendar event
// handle, and which admin walked the booking in.
type AdminBookingResponse struct {
	ID                    uint       `json:"id"`
	HomeID                uint       `json:"home_id"`
	CustomerName          string     `json:"customer_name"`
	CustomerPhone         string     `json:"customer_phone"`
	StartTime             time.Time  `json:"start_time"`
	EndTime               time.Time  `json:"end_time"`
	BookingType           string     `json:"booking_type"`
	ComputedPrice         int64      `json:"computed_price"`
	Status                string     `json:"status"`
	ExpiresAt             *time.Time `json:"expires_at"`
	CreatedAt             time.Time  `json:"created_at"`
	DoorLockCode          *string    `json:"door_lock_code"`
	LockCodeSentAt        *time.Time `json:"lock_code_sent_at"`
	GoogleCalendarEventID string     `json:"google_calendar_event_id"`
	CreatedByAdminID      *uint      `json:"created_by_admin_id"`
}

func ToAdminBookingResponse(b *model.Booking) AdminBookingResponse {
	return AdminBookingResponse{
		ID: b.ID, HomeID: b.HomeID,
		CustomerName: b.CustomerName, CustomerPhone: b.CustomerPhone,
		StartTime: b.StartTime, EndTime: b.EndTime,
		BookingType: b.BookingType, ComputedPrice: b.ComputedPrice,
		Status: b.Status, ExpiresAt: b.ExpiresAt, CreatedAt: b.CreatedAt,
		DoorLockCode: b.DoorLockCode, LockCodeSentAt: b.LockCodeSentAt,
		GoogleCalendarEventID: b.GoogleCalendarEventID,
		CreatedByAdminID:      b.CreatedByAdminID,
	}
}
