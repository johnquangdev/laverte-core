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
	Raw         string
}

// ErrWebhookPing marks a provider connectivity check rather than a payment —
// the caller must acknowledge it with 200 and settle nothing.
var ErrWebhookPing = errors.New("checkout: webhook connectivity ping")

type IPaymentProvider interface {
	CreateQR(ctx context.Context, req CreateQRRequest) (*QRResult, error)
	VerifyWebhook(ctx context.Context, raw []byte, headers http.Header) (*WebhookEvent, error)
}
