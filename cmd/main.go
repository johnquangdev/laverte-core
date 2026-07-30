package main

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/client/postgres"
	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/delivery/job"
	"github.com/johnquangdev/laverte-home/migrations"
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	pricingrulerepo "github.com/johnquangdev/laverte-home/repository/pricingrule"
	refreshtokenrepo "github.com/johnquangdev/laverte-home/repository/refreshtoken"
	userrepo "github.com/johnquangdev/laverte-home/repository/user"
	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
	bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"
	bookingjobsuc "github.com/johnquangdev/laverte-home/usecase/bookingjobs"
	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
	overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"
	pricinguc "github.com/johnquangdev/laverte-home/usecase/pricing"
	pricingadminuc "github.com/johnquangdev/laverte-home/usecase/pricingadmin"
	"github.com/johnquangdev/laverte-home/util/checkout"
	"github.com/johnquangdev/laverte-home/util/gcalendar"
	"github.com/johnquangdev/laverte-home/util/notify"
	"github.com/johnquangdev/laverte-home/util/oauth"
	"github.com/johnquangdev/laverte-home/util/ratelimit"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

func main() {
	cfg := config.GetConfig()

	log, _ := zap.NewProduction()
	defer func() { _ = log.Sync() }()

	// Resolved once, before anything serves traffic: an unparseable zone name would
	// otherwise surface as a wrong 24-hour window on an admin's first ?date= query,
	// with nothing to indicate the dates were read in the wrong clock.
	appLocation, err := time.LoadLocation(cfg.AppTimeZone)
	if err != nil {
		log.Fatal("invalid APP_TIMEZONE", zap.String("value", cfg.AppTimeZone), zap.Error(err))
	}

	if err := runMigrations(cfg, log); err != nil {
		log.Fatal("auto-migration failed", zap.Error(err))
	}

	dbFactory := postgres.GetClient

	users := userrepo.NewPG(dbFactory)
	tokens := refreshtokenrepo.NewPG(dbFactory)
	homes := homerepo.NewPG(dbFactory)
	// pricingRules also backs the booking usecase's pricinguc.New(pricingRules).
	pricingRules := pricingrulerepo.NewPG(dbFactory)
	blockedSlots := blockedslotrepo.NewPG(dbFactory)
	bookings := bookingrepo.NewPG(dbFactory)
	payments := paymentrepo.NewPG(dbFactory)

	oauthSvc := oauth.NewGoogle(cfg)
	tokenStore := tokenstore.NewRedis(cfg)
	limiter := ratelimit.NewRedis(cfg)
	sepay := checkout.NewSePay(*cfg)

	authUC := authuc.New(users, tokens, oauthSvc, tokenStore, *cfg, log)
	adminUC := adminuc.New(users)
	homeAdminUC := homeadminuc.New(homes)
	pricingAdminUC := pricingadminuc.New(pricingRules)
	blockedSlotUC := blockedslotuc.New(blockedSlots)
	pricingUC := pricinguc.New(pricingRules)
	bookingUC := bookinguc.New(bookings, homes, blockedSlots, payments, pricingUC, sepay, *cfg, log)

	// Calendar push is best-effort per design doc §5, so a missing or invalid
	// service-account key downgrades to the noop instead of aborting boot.
	var calendarSvc = gcalendar.NewNoop()
	if cal, err := gcalendar.NewGoogle(cfg); err != nil {
		log.Warn("google calendar disabled", zap.Error(err))
	} else {
		calendarSvc = cal
	}
	notifier := notify.FromConfig(cfg, log)

	billingUC := billinguc.New(bookings, payments, sepay, notifier, calendarSvc, homes, log, *cfg)
	bookingAdminUC := bookingadminuc.New(bookings, homes, blockedSlots, payments, pricingUC, notifier, calendarSvc, log)
	overviewUC := overviewuc.New(payments, bookings)

	adminRoleResolver := jwtmw.AdminRoleResolverFunc(func(ctx context.Context, userID uint) (string, error) {
		u, err := users.GetByID(ctx, userID)
		if err != nil {
			return "", err
		}
		return u.Role, nil
	})

	srv := httpserver.NewServer(*cfg, log, httpserver.Deps{
		Limiter:           limiter,
		TokenStore:        tokenStore,
		AuthUC:            authUC,
		AdminUC:           adminUC,
		AdminRoleResolver: adminRoleResolver,
		HomeAdminUC:       homeAdminUC,
		PricingAdminUC:    pricingAdminUC,
		BlockedSlotUC:     blockedSlotUC,
		BookingUC:         bookingUC,
		BookingAdminUC:    bookingAdminUC,
		BillingUC:         billingUC,
		OverviewUC:        overviewUC,
		Location:          appLocation,
	})

	bookingJobsUC := bookingjobsuc.New(bookings, payments, notifier, log, *cfg)

	j := job.New(bookingJobsUC, log)
	j.Start()
	defer j.Stop()

	log.Info("starting server", zap.String("port", cfg.Port))
	if err := srv.Start(); err != nil {
		log.Fatal("server error", zap.Error(err))
	}
}

func runMigrations(cfg *config.Config, log *zap.Logger) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Error("migrate: close db", zap.Error(cerr))
		}
	}()

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	n, err := migrate.Exec(db, "postgres", src, migrate.Up)
	if err != nil {
		return err
	}
	log.Info("migrations applied", zap.Int("count", n))
	return nil
}
