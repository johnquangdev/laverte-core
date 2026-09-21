package booking

import (
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-core/config"
	blockedslotrepo "github.com/johnquangdev/laverte-core/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-core/repository/booking"
	homerepo "github.com/johnquangdev/laverte-core/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-core/repository/payment"
	pricinguc "github.com/johnquangdev/laverte-core/usecase/pricing"
	"github.com/johnquangdev/laverte-core/util/checkout"
)

type UseCase struct {
	bookingRepo     bookingrepo.IRepository
	homeRepo        homerepo.IRepository
	blockedSlotRepo blockedslotrepo.IRepository
	paymentRepo     paymentrepo.IRepository
	pricingUC       pricinguc.IUseCase
	payment         checkout.IPaymentProvider
	cfg             config.Config
	log             *zap.Logger
}

func New(
	bookingRepo bookingrepo.IRepository,
	homeRepo homerepo.IRepository,
	blockedSlotRepo blockedslotrepo.IRepository,
	paymentRepo paymentrepo.IRepository,
	pricingUC pricinguc.IUseCase,
	payment checkout.IPaymentProvider,
	cfg config.Config,
	log *zap.Logger,
) IUseCase {
	return &UseCase{
		bookingRepo:     bookingRepo,
		homeRepo:        homeRepo,
		blockedSlotRepo: blockedSlotRepo,
		paymentRepo:     paymentRepo,
		pricingUC:       pricingUC,
		payment:         payment,
		cfg:             cfg,
		log:             log,
	}
}
