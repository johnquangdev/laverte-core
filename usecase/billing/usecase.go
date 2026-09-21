package billing

import (
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-core/config"
	bookingrepo "github.com/johnquangdev/laverte-core/repository/booking"
	homerepo "github.com/johnquangdev/laverte-core/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-core/repository/payment"
	"github.com/johnquangdev/laverte-core/util/checkout"
	"github.com/johnquangdev/laverte-core/util/gcalendar"
	"github.com/johnquangdev/laverte-core/util/notify"
)

type UseCase struct {
	bookingRepo bookingrepo.IRepository
	paymentRepo paymentrepo.IRepository
	payment     checkout.IPaymentProvider
	notifier    notify.INotifier
	calendar    gcalendar.ICalendar
	homeRepo    homerepo.IRepository
	log         *zap.Logger
	cfg         config.Config
}

func New(
	bookingRepo bookingrepo.IRepository,
	paymentRepo paymentrepo.IRepository,
	payment checkout.IPaymentProvider,
	notifier notify.INotifier,
	calendar gcalendar.ICalendar,
	homeRepo homerepo.IRepository,
	log *zap.Logger,
	cfg config.Config,
) IUseCase {
	return &UseCase{
		bookingRepo: bookingRepo,
		paymentRepo: paymentRepo,
		payment:     payment,
		notifier:    notifier,
		calendar:    calendar,
		homeRepo:    homeRepo,
		log:         log,
		cfg:         cfg,
	}
}
