package bookingadmin

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
)

func TestSetLockCodeRejectsNonConfirmedBooking(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment})

	err := uc.SetLockCode(context.Background(), b.ID, "1234")
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("SetLockCode() error = %v, want CodeValidation", err)
	}
	if b.DoorLockCode != nil {
		t.Errorf("DoorLockCode = %v, want nil", b.DoorLockCode)
	}
}

func TestSetLockCodeStoresCode(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed})

	if err := uc.SetLockCode(context.Background(), b.ID, "4321"); err != nil {
		t.Fatalf("SetLockCode() error = %v", err)
	}
	if b.DoorLockCode == nil || *b.DoorLockCode != "4321" {
		t.Errorf("DoorLockCode = %v, want 4321", b.DoorLockCode)
	}
}

func TestSendLockCodeRejectsMissingCode(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed})

	err := uc.SendLockCode(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("SendLockCode() error = %v, want CodeValidation", err)
	}
	if d.notifier.lockCodeCalls != 0 {
		t.Errorf("notifier.LockCode calls = %d, want 0", d.notifier.lockCodeCalls)
	}
}

func TestSendLockCodeSendsOnlyOnce(t *testing.T) {
	uc, d := newTestUseCase()
	code := "1357"
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, DoorLockCode: &code})

	if err := uc.SendLockCode(context.Background(), b.ID); err != nil {
		t.Fatalf("first SendLockCode() error = %v", err)
	}
	if b.LockCodeSentAt == nil {
		t.Fatal("LockCodeSentAt = nil after send, want a timestamp")
	}
	firstSentAt := *b.LockCodeSentAt

	if err := uc.SendLockCode(context.Background(), b.ID); err != nil {
		t.Fatalf("second SendLockCode() error = %v", err)
	}
	if d.notifier.lockCodeCalls != 1 {
		t.Errorf("notifier.LockCode calls = %d, want 1", d.notifier.lockCodeCalls)
	}
	if !b.LockCodeSentAt.Equal(firstSentAt) {
		t.Errorf("LockCodeSentAt changed on second call: %v -> %v, want unchanged", firstSentAt, b.LockCodeSentAt)
	}
}

func TestSendLockCodePropagatesNotifierError(t *testing.T) {
	uc, d := newTestUseCase()
	code := "2468"
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, DoorLockCode: &code})
	d.notifier.lockCodeErr = errors.New("zns down")

	if err := uc.SendLockCode(context.Background(), b.ID); err == nil {
		t.Fatal("SendLockCode() = nil error, want the notifier failure surfaced to the admin")
	}
	if b.LockCodeSentAt != nil {
		t.Errorf("LockCodeSentAt = %v, want nil so a retry can still deliver", b.LockCodeSentAt)
	}
}
