package paymentadmin

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
	ledgerrepo "github.com/johnquangdev/laverte-core/repository/ledger"
)

// fakeLedger models RefundIfStayEnded's WHERE clause, not just its happy path:
// the refund lands only while the payment is paid and its stay has ended.
type fakeLedger struct {
	rows map[uint]*ledgerrepo.PaymentRow
	// beforeRefund runs between the usecase's read and its guarded write, to
	// stand in for a second admin acting on the same payment.
	beforeRefund func()
	refundCalls  int
}

func (f *fakeLedger) ListPayments(context.Context, time.Time, time.Time, int) ([]*ledgerrepo.PaymentRow, error) {
	return nil, nil
}

func (f *fakeLedger) GetPayment(_ context.Context, id uint) (*ledgerrepo.PaymentRow, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *row
	return &cp, nil
}

func (f *fakeLedger) RefundIfStayEnded(_ context.Context, id, adminID uint, note string, at time.Time) (bool, error) {
	f.refundCalls++
	if f.beforeRefund != nil {
		f.beforeRefund()
	}
	row, ok := f.rows[id]
	if !ok || row.Status != model.PaymentStatusPaid || !stayEnded(row.BookingStatus) {
		return false, nil
	}
	row.Status = model.PaymentStatusRefunded
	row.RefundedAt = &at
	row.RefundedByAdminID = &adminID
	row.RefundNote = note
	return true, nil
}

type fakeUnmatched struct {
	rows map[uint]*model.UnmatchedTransfer
}

func (f *fakeUnmatched) RecordIfNew(context.Context, *model.UnmatchedTransfer) (bool, error) {
	return false, nil
}

func (f *fakeUnmatched) GetByID(_ context.Context, id uint) (*model.UnmatchedTransfer, error) {
	t, ok := f.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeUnmatched) List(context.Context, bool, int) ([]*model.UnmatchedTransfer, error) {
	return nil, nil
}

func (f *fakeUnmatched) ResolveIfOpen(_ context.Context, id, adminID uint, note string, at time.Time) (bool, error) {
	t, ok := f.rows[id]
	if !ok || t.ResolvedAt != nil {
		return false, nil
	}
	t.ResolvedAt = &at
	t.ResolvedByAdminID = &adminID
	t.ResolutionNote = note
	return true, nil
}

func paidRow(bookingStatus string) *ledgerrepo.PaymentRow {
	now := time.Now()
	return &ledgerrepo.PaymentRow{
		Payment: model.Payment{
			ID: 7, BookingID: 42, Provider: model.PaymentProviderSePay,
			Amount: 500000, Status: model.PaymentStatusPaid, PaidAt: &now,
		},
		CustomerName: "Khach A", HomeID: 1, BookingStatus: bookingStatus,
	}
}

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("error = %v, want apperr with Code %q", err, code)
	}
}

func TestRefundRecordsReturnForCancelledStay(t *testing.T) {
	ledger := &fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{7: paidRow(model.BookingStatusCancelled)}}
	uc := New(ledger, &fakeUnmatched{})

	resp, err := uc.Refund(context.Background(), 7, 3, "  CK hoan VCB 0123, ma FT999  ")
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if resp.Status != model.PaymentStatusRefunded || resp.RefundedAt == nil {
		t.Errorf("resp status = %q, refunded_at = %v, want refunded with a timestamp", resp.Status, resp.RefundedAt)
	}
	if resp.RefundNote != "CK hoan VCB 0123, ma FT999" {
		t.Errorf("RefundNote = %q, want it trimmed", resp.RefundNote)
	}
}

func TestRefundAllowsExpiredStay(t *testing.T) {
	ledger := &fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{7: paidRow(model.BookingStatusExpired)}}
	if _, err := New(ledger, &fakeUnmatched{}).Refund(context.Background(), 7, 3, "hoan"); err != nil {
		t.Fatalf("Refund() on an expired stay error = %v, want success", err)
	}
}

// Completed and no-show stays are revenue; confirmed must be cancelled first.
func TestRefundRefusesStayThatHasNotEnded(t *testing.T) {
	for _, status := range []string{
		model.BookingStatusConfirmed, model.BookingStatusCompleted, model.BookingStatusNoShow,
	} {
		t.Run(status, func(t *testing.T) {
			ledger := &fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{7: paidRow(status)}}
			_, err := New(ledger, &fakeUnmatched{}).Refund(context.Background(), 7, 3, "hoan")
			wantCode(t, err, apperr.CodeValidation)
			if ledger.refundCalls != 0 {
				t.Errorf("RefundIfStayEnded calls = %d, want 0", ledger.refundCalls)
			}
		})
	}
}

func TestRefundRefusesPaymentThatIsNotPaid(t *testing.T) {
	row := paidRow(model.BookingStatusCancelled)
	row.Status = model.PaymentStatusExpired
	ledger := &fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{7: row}}
	_, err := New(ledger, &fakeUnmatched{}).Refund(context.Background(), 7, 3, "hoan")
	wantCode(t, err, apperr.CodeValidation)
}

func TestRefundRequiresNote(t *testing.T) {
	ledger := &fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{7: paidRow(model.BookingStatusCancelled)}}
	_, err := New(ledger, &fakeUnmatched{}).Refund(context.Background(), 7, 3, "   ")
	wantCode(t, err, apperr.CodeValidation)
}

func TestRefundUnknownPaymentIsNotFound(t *testing.T) {
	_, err := New(&fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{}}, &fakeUnmatched{}).
		Refund(context.Background(), 99, 3, "hoan")
	wantCode(t, err, apperr.CodeNotFound)
}

// A second admin refunding between our read and our write must surface as a
// conflict, not as a success for a refund this caller did not record.
func TestRefundLosingTheRaceIsAConflict(t *testing.T) {
	ledger := &fakeLedger{rows: map[uint]*ledgerrepo.PaymentRow{7: paidRow(model.BookingStatusCancelled)}}
	ledger.beforeRefund = func() { ledger.rows[7].Status = model.PaymentStatusRefunded }
	_, err := New(ledger, &fakeUnmatched{}).Refund(context.Background(), 7, 3, "hoan")
	wantCode(t, err, apperr.CodeConflict)
}

func TestResolveUnmatchedClosesOpenTransfer(t *testing.T) {
	unmatched := &fakeUnmatched{rows: map[uint]*model.UnmatchedTransfer{
		5: {ID: 5, SePayTransactionRef: "TXN-1", Amount: 980000, Reason: model.UnmatchedReasonNoMemo},
	}}
	resp, err := New(&fakeLedger{}, unmatched).ResolveUnmatched(context.Background(), 5, 3, "da hoan cho khach")
	if err != nil {
		t.Fatalf("ResolveUnmatched() error = %v", err)
	}
	if resp.ResolvedAt == nil || resp.ResolutionNote != "da hoan cho khach" {
		t.Errorf("resp = %+v, want resolved with the note", resp)
	}
}

func TestResolveUnmatchedRefusesAlreadyResolved(t *testing.T) {
	at := time.Now()
	admin := uint(1)
	unmatched := &fakeUnmatched{rows: map[uint]*model.UnmatchedTransfer{
		5: {ID: 5, ResolvedAt: &at, ResolvedByAdminID: &admin, ResolutionNote: "first"},
	}}
	_, err := New(&fakeLedger{}, unmatched).ResolveUnmatched(context.Background(), 5, 3, "second")
	wantCode(t, err, apperr.CodeValidation)
	if unmatched.rows[5].ResolutionNote != "first" {
		t.Errorf("ResolutionNote = %q, want the first admin's note kept", unmatched.rows[5].ResolutionNote)
	}
}

func TestResolveUnmatchedRequiresNote(t *testing.T) {
	unmatched := &fakeUnmatched{rows: map[uint]*model.UnmatchedTransfer{5: {ID: 5}}}
	_, err := New(&fakeLedger{}, unmatched).ResolveUnmatched(context.Background(), 5, 3, "")
	wantCode(t, err, apperr.CodeValidation)
}
