package model

import "time"

const (
	BookingTypeHourly    = "hourly"
	BookingTypeOvernight = "overnight"
	BookingTypeDay       = "day"

	BookingStatusPendingPayment = "pending_payment"
	BookingStatusConfirmed      = "confirmed"
	BookingStatusCancelled      = "cancelled"
	BookingStatusExpired        = "expired"
	BookingStatusCompleted      = "completed"
	BookingStatusNoShow         = "no_show"
)

type Booking struct {
	ID                    uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	HomeID                uint       `gorm:"not null;index" json:"home_id"`
	CustomerName          string     `gorm:"not null" json:"customer_name"`
	CustomerPhone         string     `gorm:"not null;index" json:"customer_phone"`
	StartTime             time.Time  `gorm:"not null" json:"start_time"`
	EndTime               time.Time  `gorm:"not null" json:"end_time"`
	BookingType           string     `gorm:"not null" json:"booking_type"`
	ComputedPrice         int64      `gorm:"not null" json:"computed_price"`
	Status                string     `gorm:"not null;index;default:'pending_payment'" json:"status"`
	PaymentID             *uint      `json:"payment_id"`
	GoogleCalendarEventID string     `json:"google_calendar_event_id"`
	DoorLockCode          *string    `json:"door_lock_code"`
	LockCodeAlertSentAt   *time.Time `json:"lock_code_alert_sent_at"`
	LockCodeSentAt        *time.Time `json:"lock_code_sent_at"`
	CreatedByAdminID      *uint      `json:"created_by_admin_id"`
	ExpiresAt             *time.Time `json:"expires_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (Booking) TableName() string { return "bookings" }

func IsValidBookingType(t string) bool {
	return t == BookingTypeHourly || t == BookingTypeOvernight || t == BookingTypeDay
}
