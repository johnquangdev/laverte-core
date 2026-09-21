package main

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-core/client/postgres"
	"github.com/johnquangdev/laverte-core/config"
	httpserver "github.com/johnquangdev/laverte-core/delivery/http"
	jwtmw "github.com/johnquangdev/laverte-core/delivery/http/middleware"
	"github.com/johnquangdev/laverte-core/delivery/job"
	"github.com/johnquangdev/laverte-core/migrations"
	blockedslotrepo "github.com/johnquangdev/laverte-core/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-core/repository/booking"
	homerepo "github.com/johnquangdev/laverte-core/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-core/repository/payment"
	pricingrulerepo "github.com/johnquangdev/laverte-core/repository/pricingrule"
	refreshtokenrepo "github.com/johnquangdev/laverte-core/repository/refreshtoken"
	userrepo "github.com/johnquangdev/laverte-core/repository/user"
	adminuc "github.com/johnquangdev/laverte-core/usecase/admin"
	authuc "github.com/johnquangdev/laverte-core/usecase/auth"
	billinguc "github.com/johnquangdev/laverte-core/usecase/billing"
	blockedslotuc "github.com/johnquangdev/laverte-core/usecase/blockedslot"
	bookinguc "github.com/johnquangdev/laverte-core/usecase/booking"
	bookingadminuc "github.com/johnquangdev/laverte-core/usecase/bookingadmin"
	bookingjobsuc "github.com/johnquangdev/laverte-core/usecase/bookingjobs"
	homeadminuc "github.com/johnquangdev/laverte-core/usecase/homeadmin"
	overviewuc "github.com/johnquangdev/laverte-core/usecase/overview"
	pricinguc "github.com/johnquangdev/laverte-core/usecase/pricing"
	pricingadminuc "github.com/johnquangdev/laverte-core/usecase/pricingadmin"
	"github.com/johnquangdev/laverte-core/util/checkout"
	"github.com/johnquangdev/laverte-core/util/gcalendar"
	"github.com/johnquangdev/laverte-core/util/notify"
	"github.com/johnquangdev/laverte-core/util/oauth"
	"github.com/johnquangdev/laverte-core/util/ratelimit"
	"github.com/johnquangdev/laverte-core/util/tokenstore"
)

func main() {
	cfg := config.GetConfig()

	log, _ := zap.NewProduction()
	defer func() { _ = log.Sync() }()

	if err := cfg.Validate(); err != nil {
		log.Fatal("invalid configuration", zap.Error(err))
	}

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
