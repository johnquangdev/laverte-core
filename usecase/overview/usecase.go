package overview

import (
	"context"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/presenter"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
)

type UseCase struct {
	paymentRepo paymentrepo.IRepository
	bookingRepo bookingrepo.IRepository
}

func New(paymentRepo paymentrepo.IRepository, bookingRepo bookingrepo.IRepository) IUseCase {
	return &UseCase{paymentRepo: paymentRepo, bookingRepo: bookingRepo}
}

func (uc *UseCase) Summary(ctx context.Context, from, to time.Time) (*presenter.OverviewResponse, error) {
	if !to.After(from) {
		return nil, apperr.Validation("to phai sau from")
	}

	// Revenue is read from paid payments, never from summing
	// booking.computed_price: a walk-in can be confirmed with cash outside the
	// quoted price, and a booking can be cancelled after it was paid, so "money
	// received" and "price quoted" are two different numbers and must not be
	// conflated in one figure.
	revenue, err := uc.paymentRepo.SumPaidBetween(ctx, from, to)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	count, err := uc.bookingRepo.CountConfirmedBetween(ctx, from, to)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	return &presenter.OverviewResponse{From: from, To: to, TotalRevenueVND: revenue, BookingCount: count}, nil
}
