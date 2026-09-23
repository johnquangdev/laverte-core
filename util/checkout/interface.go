package checkout

import (
	"context"
	"errors"
	"net/http"
)

type CreateQRRequest struct {
	BookingID uint
	AmountVND int64
}

type QRResult struct {
	QRContent   string // URL or payload the FE renders as a QR image
	ProviderRef string // memo we expect back in the bank transfer content
}

type WebhookEvent struct {
	ProviderRef string // memo parsed out of the transfer content
	ExternalRef string // SePay's own stable transaction id (dedup key)
	Success     bool
	AmountVND   int64
	// Content is the transfer text as the bank sent it, kept so an admin can
	// recognise a transfer that settled nothing.
	Content string
	Raw     string
}

// ErrWebhookPing marks a provider connectivity check rather than a payment —
// the caller must acknowledge it with 200 and settle nothing.
var ErrWebhookPing = errors.New("checkout: webhook connectivity ping")

// ErrNoBookingMemo marks an authentic transfer whose content names no booking.
// VerifyWebhook returns it together with a non-nil event, because the money is
// real and the caller must be able to record it for reconciliation.
var ErrNoBookingMemo = errors.New("checkout: no recognizable booking memo in transfer content")

type IPaymentProvider interface {
	CreateQR(ctx context.Context, req CreateQRRequest) (*QRResult, error)
	// VerifyWebhook returns a nil event on every error except ErrNoBookingMemo,
	// which comes with the authenticated event minus its ProviderRef.
	VerifyWebhook(ctx context.Context, raw []byte, headers http.Header) (*WebhookEvent, error)
}
