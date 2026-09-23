package model

import "time"

// Reasons a settled bank transfer could not be applied to a booking. Each one
// leaves real money in the account that only a human can reconcile.
const (
	UnmatchedReasonNoMemo            = "no_memo"
	UnmatchedReasonBookingNotFound   = "booking_not_found"
	UnmatchedReasonBookingNotPending = "booking_not_pending"
	UnmatchedReasonAmountMismatch    = "amount_mismatch"
)

type UnmatchedTransfer struct {
	ID                  uint   `gorm:"primaryKey;autoIncrement"`
	SePayTransactionRef string `gorm:"column:sepay_transaction_ref"`
	Amount              int64
	Content             string
	Reason              string
	// BookingRef is the id the memo named, when it named one. It is not a
	// foreign key: for UnmatchedReasonBookingNotFound it points at nothing.
	BookingRef        *uint
	ReceivedAt        time.Time
	ResolvedAt        *time.Time
	ResolvedByAdminID *uint
	ResolutionNote    string
}

func (UnmatchedTransfer) TableName() string { return "unmatched_transfers" }
