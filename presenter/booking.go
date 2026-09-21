package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

type BookingResponse struct {
	ID            uint       `json:"id"`
	HomeID        uint       `json:"home_id"`
	CustomerName  string     `json:"customer_name"`
	CustomerPhone string     `json:"customer_phone"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       time.Time  `json:"end_time"`
	BookingType   string     `json:"booking_type"`
	ComputedPrice int64      `json:"computed_price"`
	Status        string     `json:"status"`
	ExpiresAt     *time.Time `json:"expires_at"`
	QRContent     string     `json:"qr_content"`
}

// ToBookingResponse takes qrContent separately: the QR lives on the Payment
// row, and a booking can be re-served with an already-issued QR.
func ToBookingResponse(b *model.Booking, qrContent string) BookingResponse {
	return BookingResponse{
		ID: b.ID, HomeID: b.HomeID, CustomerName: b.CustomerName, CustomerPhone: b.CustomerPhone,
		StartTime: b.StartTime, EndTime: b.EndTime, BookingType: b.BookingType,
		ComputedPrice: b.ComputedPrice, Status: b.Status, ExpiresAt: b.ExpiresAt, QRContent: qrContent,
	}
}
