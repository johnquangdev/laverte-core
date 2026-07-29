package booking

import (
	"github.com/johnquangdev/laverte-home/config"
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	pricinguc "github.com/johnquangdev/laverte-home/usecase/pricing"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

type UseCase struct {
	bookingRepo     bookingrepo.IRepository
	homeRepo        homerepo.IRepository
	blockedSlotRepo blockedslotrepo.IRepository
	paymentRepo     paymentrepo.IRepository
	pricingUC       pricinguc.IUseCase
	payment         checkout.IPaymentProvider
	cfg             config.Config
}

func New(
	bookingRepo bookingrepo.IRepository,
	homeRepo homerepo.IRepository,
	blockedSlotRepo blockedslotrepo.IRepository,
	paymentRepo paymentrepo.IRepository,
	pricingUC pricinguc.IUseCase,
	payment checkout.IPaymentProvider,
	cfg config.Config,
) IUseCase {
	return &UseCase{
		bookingRepo:     bookingRepo,
		homeRepo:        homeRepo,
		blockedSlotRepo: blockedSlotRepo,
		paymentRepo:     paymentRepo,
		pricingUC:       pricingUC,
		payment:         payment,
		cfg:             cfg,
	}
}
