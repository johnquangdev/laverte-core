package bookingadmin

import (
	"context"
	"time"

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

	b.DoorLockCode = &code
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (uc *UseCase) SendLockCode(ctx context.Context, id uint) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	if b.DoorLockCode == nil {
		return apperr.Validation("chua co ma khoa")
	}
	if b.LockCodeSentAt != nil {
		return nil
	}

	// Unlike the webhook's best-effort notification, a failure here is returned:
	// an admin pressed "send now" while the guest waits at the door and must see
	// that the message did not go out. LockCodeSentAt stays nil so the cron (or a
	// retry) can still deliver it.
	if err := uc.notifier.LockCode(ctx, b, *b.DoorLockCode); err != nil {
		return apperr.Internal(err)
	}

	// One column, not Update's Save-every-column: the Task 17 sweep writes this same
	// field on a timer, and a read-modify-Save from here would overwrite its value
	// from a snapshot taken before it ran — resetting LockCodeSentAt and sending the
	// door code a second time.
	if err := uc.bookingRepo.MarkLockCodeSent(ctx, b.ID, time.Now()); err != nil {
		return apperr.Internal(err)
	}
	return nil
}
