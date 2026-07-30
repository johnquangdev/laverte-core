package bookingjobs

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	"github.com/johnquangdev/laverte-home/util/notify"
)

type UseCase struct {
	bookingRepo bookingrepo.IRepository
	paymentRepo paymentrepo.IRepository
	notifier    notify.INotifier
	log         *zap.Logger
	cfg         config.Config
}

func New(
	bookingRepo bookingrepo.IRepository,
	paymentRepo paymentrepo.IRepository,
	notifier notify.INotifier,
	log *zap.Logger,
	cfg config.Config,
) IUseCase {
	return &UseCase{bookingRepo: bookingRepo, paymentRepo: paymentRepo, notifier: notifier, log: log, cfg: cfg}
}

func (uc *UseCase) ExpirePendingBookings(ctx context.Context) error {
	rows, err := uc.bookingRepo.ListExpiredPending(ctx, time.Now())
	if err != nil {
		return apperr.Internal(err)
	}

	for _, b := range rows {
		// One unwritable row must not strand the rest of the batch: every booking
		// left pending keeps a slot unsellable, and the sweep runs again in a
		// minute, so a per-row failure is logged and retried rather than aborting.
		expired, err := uc.bookingRepo.ExpireIfPending(ctx, b.ID)
		if err != nil {
			uc.log.Error("expire booking failed", zap.Uint("booking_id", b.ID), zap.Error(err))
			continue
		}
		// The webhook confirmed this booking after the list query above. Leaving it
		// alone is the whole point of the guarded update — writing the batch snapshot
		// back would revert a paid stay to 'expired' and, because 'expired' is outside
		// the overlap exclusion constraint, reopen the slot to a different guest.
		if !expired {
			continue
		}
		uc.expirePaymentOf(ctx, b)
	}
	return nil
}

func (uc *UseCase) expirePaymentOf(ctx context.Context, b *model.Booking) {
	if b.PaymentID == nil {
		return
	}
	p, err := uc.paymentRepo.GetByBookingID(ctx, b.ID)
	if err != nil {
		uc.log.Error("load payment for expiry failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	// A paid row means the webhook won the race with this sweep; leave it alone
	// so the money stays reconciled against the booking.
	if p.Status != model.PaymentStatusPending {
		return
	}
	if err := uc.paymentRepo.MarkExpiredIfPending(ctx, p.ID); err != nil {
		uc.log.Error("expire payment failed", zap.Uint("payment_id", p.ID), zap.Error(err))
	}
}

func (uc *UseCase) AlertMissingLockCodes(ctx context.Context) error {
	leadTime := time.Duration(uc.cfg.BookingCheckinAlertLeadMinutes) * time.Minute
	rows, err := uc.bookingRepo.ListUpcomingMissingLockCode(ctx, time.Now(), leadTime)
	if err != nil {
		return apperr.Internal(err)
	}

	for _, b := range rows {
		if err := uc.notifier.AdminLockCodeMissing(ctx, b); err != nil {
			uc.log.Error("lock-code alert failed", zap.Uint("booking_id", b.ID), zap.Error(err))
			continue
		}
		// LockCodeAlertSentAt is the only thing keeping this from re-alerting on
		// every tick, so it is written only after the alert actually went out — and as
		// a single column, since the admin may be setting DoorLockCode on this same row
		// right now and a full-row Save would write it back to the value this loop read.
		if err := uc.bookingRepo.MarkLockCodeAlertSent(ctx, b.ID, time.Now()); err != nil {
			uc.log.Error("mark lock-code alert sent failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		}
	}
	return nil
}

func (uc *UseCase) SendDueLockCodes(ctx context.Context) error {
	rows, err := uc.bookingRepo.ListReadyToSendLockCode(ctx, time.Now())
	if err != nil {
		return apperr.Internal(err)
	}

	for _, b := range rows {
		if b.DoorLockCode == nil {
			uc.log.Warn("booking queued for lock-code send has no code", zap.Uint("booking_id", b.ID))
			continue
		}
		// Claim before sending. The admin's "send now" button reads the same row, so
		// deciding from this loop's snapshot and stamping afterwards would let both
		// deliver — two copies of a physical-access credential.
		claimed, err := uc.bookingRepo.ClaimLockCodeSend(ctx, b.ID, time.Now())
		if err != nil {
			uc.log.Error("claim lock-code send failed", zap.Uint("booking_id", b.ID), zap.Error(err))
			continue
		}
		if !claimed {
			continue
		}
		if err := uc.notifier.LockCode(ctx, b, *b.DoorLockCode); err != nil {
			uc.log.Error("lock-code send failed", zap.Uint("booking_id", b.ID), zap.Error(err))
			// Hand the claim back so the next tick retries; otherwise the row reads as
			// sent and the guest reaches a locked door with no code.
			if rerr := uc.bookingRepo.ReleaseLockCodeSend(ctx, b.ID); rerr != nil {
				uc.log.Error("release lock-code claim failed", zap.Uint("booking_id", b.ID), zap.Error(rerr))
			}
			continue
		}
	}
	return nil
}
