package billing

import (
	"context"
	"net/http"
)

type IUseCase interface {
	// HandleSePayWebhook takes the raw request body and headers because the
	// provider signs the exact bytes it sent.
	HandleSePayWebhook(ctx context.Context, raw []byte, headers http.Header) error
}
