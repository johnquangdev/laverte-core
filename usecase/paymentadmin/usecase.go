package paymentadmin

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	"github.com/johnquangdev/laverte-core/presenter"
	ledgerrepo "github.com/johnquangdev/laverte-core/repository/ledger"
	unmatchedtransferrepo "github.com/johnquangdev/laverte-core/repository/unmatchedtransfer"
)

// listLimit caps one page. A small property takes a few hundred payments a
// month; a range wider than that is a request for an export, not a screen.
const listLimit = 500

type UseCase struct {
	ledger    ledgerrepo.IRepository
	unmatched unmatchedtransferrepo.IRepository
}

func New(ledger ledgerrepo.IRepository, unmatched unmatchedtransferrepo.IRepository) IUseCase {
	return &UseCase{ledger: ledger, unmatched: unmatched}
}

func toResponse(row *ledgerrepo.PaymentRow) presenter.AdminPaymentResponse {
	return presenter.ToAdminPaymentResponse(&row.Payment, row.CustomerName, row.CustomerPhone, row.HomeID, row.BookingStatus)
}

func (uc *UseCase) List(ctx context.Context, from, to time.Time) ([]presenter.AdminPaymentResponse, error) {
	if !to.After(from) {
		return nil, apperr.Validation("to phai sau from")
	}
	rows, err := uc.ledger.ListPayments(ctx, from, to, listLimit)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.AdminPaymentResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResponse(row))
	}
	return out, nil
}

// stayEnded reports whether a booking will never be stayed, which is the only
// case where money paid for it must go back. Completed and no-show are revenue.
func stayEnded(status string) bool {
	return status == model.BookingStatusCancelled || status == model.BookingStatusExpired
}

func (uc *UseCase) Refund(ctx context.Context, paymentID, adminID uint, note string) (*presenter.AdminPaymentResponse, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil, apperr.Validation("nhap ghi chu hoan tien, vi du so tai khoan va ma giao dich hoan")
	}
	row, err := uc.ledger.GetPayment(ctx, paymentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NotFound(err)
		}
		return nil, apperr.Internal(err)
	}
	if row.Status != model.PaymentStatusPaid {
		return nil, apperr.Validation("chi giao dich da thu moi hoan tien duoc")
	}
	if !stayEnded(row.BookingStatus) {
		return nil, apperr.Validation("chi hoan tien khi booking da huy hoac het han — huy booking truoc")
	}

	refunded, err := uc.ledger.RefundIfStayEnded(ctx, paymentID, adminID, note, time.Now())
	if err != nil {
		return nil, apperr.Internal(err)
	}
	// The checks above ran on a row read moments ago; the guarded UPDATE is what
	// enforces them. Losing means another admin refunded it first.
	if !refunded {
		return nil, apperr.Conflict(nil, "giao dich vua thay doi trang thai, tai lai trang de xem")
	}

	row, err = uc.ledger.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	resp := toResponse(row)
	return &resp, nil
}

func (uc *UseCase) ListUnmatched(ctx context.Context, openOnly bool) ([]presenter.UnmatchedTransferResponse, error) {
	rows, err := uc.unmatched.List(ctx, openOnly, listLimit)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.UnmatchedTransferResponse, 0, len(rows))
	for _, t := range rows {
		out = append(out, presenter.ToUnmatchedTransferResponse(t))
	}
	return out, nil
}

func (uc *UseCase) ResolveUnmatched(ctx context.Context, id, adminID uint, note string) (*presenter.UnmatchedTransferResponse, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil, apperr.Validation("nhap ghi chu cach da xu ly khoan tien nay")
	}
	t, err := uc.unmatched.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NotFound(err)
		}
		return nil, apperr.Internal(err)
	}
	if t.ResolvedAt != nil {
		return nil, apperr.Validation("khoan tien nay da duoc xu ly")
	}
	resolved, err := uc.unmatched.ResolveIfOpen(ctx, id, adminID, note, time.Now())
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if !resolved {
		return nil, apperr.Conflict(nil, "khoan tien nay vua duoc admin khac xu ly, tai lai trang de xem")
	}
	t, err = uc.unmatched.GetByID(ctx, id)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToUnmatchedTransferResponse(t)
	return &resp, nil
}
