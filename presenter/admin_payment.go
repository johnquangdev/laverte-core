package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

// AdminPaymentResponse is a payment with the booking fields that let an admin
// recognise it. BookingStatus is included because it decides whether a paid
// row can be refunded: only a stay that will not happen gives money back.
type AdminPaymentResponse struct {
	ID                  uint       `json:"id"`
	BookingID           uint       `json:"booking_id"`
	CustomerName        string     `json:"customer_name"`
	CustomerPhone       string     `json:"customer_phone"`
	HomeID              uint       `json:"home_id"`
	BookingStatus       string     `json:"booking_status"`
	Provider            string     `json:"provider"`
	Amount              int64      `json:"amount"`
	Status              string     `json:"status"`
	QRContent           string     `json:"qr_content"`
	SePayTransactionRef string     `json:"sepay_transaction_ref"`
	PaidAt              *time.Time `json:"paid_at"`
	RefundedAt          *time.Time `json:"refunded_at"`
	RefundNote          string     `json:"refund_note"`
	CreatedAt           time.Time  `json:"created_at"`
}

func ToAdminPaymentResponse(p *model.Payment, customerName, customerPhone string, homeID uint, bookingStatus string) AdminPaymentResponse {
	return AdminPaymentResponse{
		ID: p.ID, BookingID: p.BookingID,
		CustomerName: customerName, CustomerPhone: customerPhone,
		HomeID: homeID, BookingStatus: bookingStatus,
		Provider: p.Provider, Amount: p.Amount, Status: p.Status,
		QRContent: p.QRContent, SePayTransactionRef: p.SePayTransactionRef,
		PaidAt: p.PaidAt, RefundedAt: p.RefundedAt, RefundNote: p.RefundNote,
		CreatedAt: p.CreatedAt,
	}
}

type UnmatchedTransferResponse struct {
	ID                  uint       `json:"id"`
	SePayTransactionRef string     `json:"sepay_transaction_ref"`
	Amount              int64      `json:"amount"`
	Content             string     `json:"content"`
	Reason              string     `json:"reason"`
	BookingRef          *uint      `json:"booking_ref"`
	ReceivedAt          time.Time  `json:"received_at"`
	ResolvedAt          *time.Time `json:"resolved_at"`
	ResolvedByAdminID   *uint      `json:"resolved_by_admin_id"`
	ResolutionNote      string     `json:"resolution_note"`
}

func ToUnmatchedTransferResponse(t *model.UnmatchedTransfer) UnmatchedTransferResponse {
	return UnmatchedTransferResponse{
		ID: t.ID, SePayTransactionRef: t.SePayTransactionRef, Amount: t.Amount,
		Content: t.Content, Reason: t.Reason, BookingRef: t.BookingRef,
		ReceivedAt: t.ReceivedAt, ResolvedAt: t.ResolvedAt,
		ResolvedByAdminID: t.ResolvedByAdminID, ResolutionNote: t.ResolutionNote,
	}
}
