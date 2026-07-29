package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/client/postgres"
	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
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
	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
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
	// Both channels are required together: sending the guest's ZNS while the
	// admin alert has nowhere to go (or the reverse) hides a misconfiguration
	// behind half-working notifications, so a partial setup stays on the noop.
	var notifier = notify.NewNoop()
	if cfg.ZNSAccessToken != "" && cfg.SMTPHost != "" {
		notifier = notify.NewComposite(notify.NewZNS(cfg, &http.Client{Timeout: 10 * time.Second}), notify.NewSMTPAdmin(cfg))
	} else {
		log.Warn("notifications disabled",
			zap.Bool("zns_configured", cfg.ZNSAccessToken != ""),
			zap.Bool("smtp_configured", cfg.SMTPHost != ""))
	}

	billingUC := billinguc.New(bookings, payments, sepay, notifier, calendarSvc, homes, log, *cfg)

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
		BillingUC:         billingUC,
	})
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
