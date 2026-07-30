package bookingadmin

import (
	"context"
	"time"

	"go.uber.org/zap"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
)

func (uc *UseCase) SetLockCode(ctx context.Context, id uint, code string) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	if b.Status != model.BookingStatusConfirmed {
		return apperr.Validation("chi booking dang 'confirmed' moi nhap duoc ma khoa")
	}

	// One column, not Update's Save-every-column: the Task 17 alert sweep writes
	// LockCodeAlertSentAt on a timer, and a read-modify-Save from here would reset it
	// from a snapshot taken before the sweep ran, re-alerting the admin.
	if err := uc.bookingRepo.SetDoorLockCode(ctx, b.ID, code); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (uc *UseCase) SendLockCode(ctx context.Context, id uint) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	// Cancelling a booking does not clear DoorLockCode, so without this check the
	// code could still be sent to a guest whose stay was cancelled — handing
	// physical access to the property to someone with no booking.
	if b.Status != model.BookingStatusConfirmed {
		return apperr.Validation("chi gui duoc ma khoa cho booking dang 'confirmed'")
	}
	if b.DoorLockCode == nil {
		return apperr.Validation("chua co ma khoa")
	}

	// Claim first, then send. Checking LockCodeSentAt above and stamping it after the
	// send would leave a window where a double-clicked button, or this call racing the
	// Task 17 sweep, both see NULL and both deliver the code.
	claimed, err := uc.bookingRepo.ClaimLockCodeSend(ctx, b.ID, time.Now())
	if err != nil {
		return apperr.Internal(err)
	}
	if !claimed {
		return nil // already sent, or another in-flight send won the claim
	}

	// Unlike the webhook's best-effort notification, a failure here is returned: an
	// admin pressed "send now" while the guest waits at the door and must see that
	// the message did not go out. Release the claim so a retry — or the sweep — can
	// still deliver; otherwise the booking would look sent while nothing arrived.
	if err := uc.notifier.LockCode(ctx, b, *b.DoorLockCode); err != nil {
		if rerr := uc.bookingRepo.ReleaseLockCodeSend(ctx, b.ID); rerr != nil {
			uc.log.Error("lock-code send failed and the claim could not be released",
				zap.Uint("booking_id", b.ID), zap.Error(rerr))
		}
		return apperr.Internal(err)
	}
	return nil
}
