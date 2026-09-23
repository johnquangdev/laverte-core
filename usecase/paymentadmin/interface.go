package paymentadmin

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/presenter"
)

type IUseCase interface {
	// List returns payments created in [from, to), newest first.
	List(ctx context.Context, from, to time.Time) ([]presenter.AdminPaymentResponse, error)
	// Refund records that a paid payment was returned by hand. Only a payment
	// whose booking was cancelled or expired qualifies.
	Refund(ctx context.Context, paymentID, adminID uint, note string) (*presenter.AdminPaymentResponse, error)
	ListUnmatched(ctx context.Context, openOnly bool) ([]presenter.UnmatchedTransferResponse, error)
	ResolveUnmatched(ctx context.Context, id, adminID uint, note string) (*presenter.UnmatchedTransferResponse, error)
}
