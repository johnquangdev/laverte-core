package model

import "time"

const (
	PaymentProviderSePay = "sepay"
	PaymentProviderCash  = "cash"

	PaymentStatusPending = "pending"
	PaymentStatusPaid    = "paid"
	PaymentStatusExpired = "expired"
	PaymentStatusFailed  = "failed"
)

type Payment struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	BookingID uint   `gorm:"not null;index" json:"booking_id"`
	Provider  string `gorm:"not null" json:"provider"`
	Amount    int64  `gorm:"not null" json:"amount"`
	Status    string `gorm:"not null;index;default:'pending'" json:"status"`
	QRContent string `gorm:"column:qr_content" json:"qr_content"`
	// SePayTransactionRef holds SePay's own transaction id, stable across
	// webhook redeliveries — it is the dedup key, not the transfer memo.
	SePayTransactionRef string     `gorm:"column:sepay_transaction_ref" json:"sepay_transaction_ref"`
	PaidAt              *time.Time `json:"paid_at"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (Payment) TableName() string { return "payments" }
