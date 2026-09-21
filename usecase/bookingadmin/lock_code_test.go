package bookingadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	apperr "github.com/johnquangdev/laverte-core/errors"
	"github.com/johnquangdev/laverte-core/model"
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
	if d.bookings.setCodeCalls != 1 {
		t.Errorf("setCodeCalls = %d, want 1: the code must be written as its own column", d.bookings.setCodeCalls)
	}
}

// A full-row Save from a snapshot read before the alert sweep ran would silently
// reset LockCodeAlertSentAt and re-alert the admin. afterGetByID fires the sweep's
// write on the stored row after SetLockCode's own read has already taken its
// (unalerted) snapshot, so a regression to Update would overwrite it with nil — a
// value assertion taken before the read starts would miss that race entirely.
func TestSetLockCodeLeavesAlertTimestampAlone(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed})

	alerted := time.Now().Add(-time.Hour)
	d.bookings.afterGetByID = func(stored *model.Booking) { stored.LockCodeAlertSentAt = &alerted }

	if err := uc.SetLockCode(context.Background(), b.ID, "4321"); err != nil {
		t.Fatalf("SetLockCode() error = %v", err)
	}
	stored := d.bookings.rows[b.ID]
	if stored.LockCodeAlertSentAt == nil || !stored.LockCodeAlertSentAt.Equal(alerted) {
		t.Errorf("LockCodeAlertSentAt = %v, want %v unchanged", stored.LockCodeAlertSentAt, alerted)
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
		t.Errorf("LockCodeSentAt moved %v -> %v; the second call must not re-stamp", firstSentAt, b.LockCodeSentAt)
	}
	// One claim: a door code is a physical-access credential, so the send must be
	// gated by the DB claim rather than by a read-then-write.
	if d.bookings.claimCalls != 1 {
		t.Errorf("claimCalls = %d, want 1", d.bookings.claimCalls)
	}
}

// The admin's send races the Task 17 sweep. Whoever loses the claim must not send a
// second copy of the code, even though it read LockCodeSentAt as nil.
func TestSendLockCodeSkipsWhenClaimLost(t *testing.T) {
	uc, d := newTestUseCase()
	code := "1357"
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, DoorLockCode: &code})

	sentAt := time.Now().Add(-time.Minute)
	d.bookings.beforeClaim = func() { b.LockCodeSentAt = &sentAt }

	if err := uc.SendLockCode(context.Background(), b.ID); err != nil {
		t.Fatalf("SendLockCode() error = %v", err)
	}
	if d.notifier.lockCodeCalls != 0 {
		t.Errorf("notifier.LockCode calls = %d, want 0 — the claim was lost", d.notifier.lockCodeCalls)
	}
}

func TestSendLockCodeRejectsCancelledBooking(t *testing.T) {
	uc, d := newTestUseCase()
	code := "9999"
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusCancelled, DoorLockCode: &code})

	err := uc.SendLockCode(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("SendLockCode() error = %v, want CodeValidation", err)
	}
	if d.notifier.lockCodeCalls != 0 {
		t.Errorf("notifier.LockCode calls = %d, want 0 — a cancelled stay must not receive the door code", d.notifier.lockCodeCalls)
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
	if d.bookings.releaseCalls != 1 {
		t.Errorf("releaseCalls = %d, want 1: the claim must be handed back or the booking looks sent while nothing arrived",
			d.bookings.releaseCalls)
	}
}
