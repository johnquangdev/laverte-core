# laverte-home Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the laverte-home Go backend — homestay booking (hourly/overnight/day pricing by category), SePay VietQR payment, Google Calendar push, Zalo ZNS + email notifications, JWT+Google-OAuth auth, admin API — from an empty repo.

**Architecture:** Clean architecture ported/adapted from `lumen-app-backend` (`delivery → usecase → repository → model`), Echo + GORM/pgx + Redis + sql-migrate + zap. See `docs/superpowers/specs/2026-07-29-laverte-home-backend-design.md`.

**Tech Stack:** Go 1.25, Echo v4, GORM + `gorm.io/driver/postgres`, `jackc/pgx/v5`, Redis (`redis/go-redis/v9`), `rubenv/sql-migrate`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `google.golang.org/api/calendar/v3`, `robfig/cron/v3`, `go.uber.org/zap`, `kelseyhightower/envconfig`, `joho/godotenv`.

**Shape:** 18 tasks in 5 phases. Phase 0 (1-6) foundation + auth; Phase 1 (7-9) home/pricing/booking domain; Phase 2 (10-13) blocked slots, SePay rail, guest booking + webhook; Phase 3 (14-15) Calendar/ZNS/email adapters; Phase 4 (16-18) admin ops, cron jobs, revenue + final verification.

## Global Constraints

- Go module: `github.com/johnquangdev/laverte-home`. Go 1.25.
- Only SePay VietQR bank-transfer is implemented — no MoMo/VNPay/PayOS/SePay-PG, no multi-provider Router/Pool/Breaker.
- No Plan/Subscription/Credits concepts anywhere.
- No phone OTP verification for booking.
- Booking conflict-checking is enforced by a Postgres exclusion constraint (`btree_gist` over `tstzrange`), not app-code locking.
- All money amounts are `int64` VND (no decimals).
- Every usecase error surfaced to HTTP goes through the `errors` (apperr) package — no bare `errors.New` reaching a handler. Authorization 403s are the exception: middleware answers those directly.
- `NewServer` takes its dependencies in a named `httpserver.Deps` struct, never as positional parameters. Almost every task adds one field; a named field can be appended without shifting any existing call site.
- Repositories are always constructed as `NewPG(getDB func(context.Context) *gorm.DB) IRepository`.
- Each `delivery/http/<domain>` package declares its own `HandleErrFunc`/`HandleOKFunc`. `delivery/http/admin` declares them once in Task 6 — later files added to that package must not redeclare them.
- `docker-compose.dev.yml` provides local Postgres (with `btree_gist` available) + Redis. Integration tests read `TEST_DATABASE_URL` and SKIP when it is unset.
- Price changes never mutate a live `PricingRule` in place — they close it (`effective_to`, `is_active=false`) and insert a replacement, so the price a past booking was charged under stays reconstructible.

---

## Phase 0 — Foundation

### Task 1: Module skeleton, config, apperr, minimal HTTP server

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `.env.example`
- Create: `config/config.go`
- Create: `config/config_test.go`
- Create: `errors/errors.go`
- Create: `errors/errors_test.go`
- Create: `delivery/http/http.go`
- Create: `delivery/http/http_test.go`
- Create: `cmd/main.go`

**Interfaces:**
- Produces: `config.Config` struct (all fields used anywhere in this plan — see below), `config.GetConfig() *Config`, `(*Config).DatabaseURL() string`.
- Produces: `apperr.Error{Code, CodeID, HTTPCode, Message, Raw}`, `apperr.As(err) (*Error, bool)`, constructors `apperr.Internal`, `apperr.NotFound`, `apperr.Validation`, `apperr.Unauthorized`, `apperr.SlotConflict`, `apperr.BookingExpired`, `apperr.AmountMismatch`. There is deliberately no `Forbidden` constructor: authorization is enforced by middleware (`RequireAdmin`/`RequireSuperAdmin`), which answers 403 itself, so no usecase ever raises one.
- Produces: `httpserver.NewServer(cfg config.Config, log *zap.Logger) *httpserver.Server`, `(*Server).Echo() *echo.Echo`, `(*Server).Start() error`.

- [ ] **Step 1: Init module and add dependencies**

```bash
cd /Users/gunnguyen/go/src/github.com/johnquangdev/laverte-home
go mod init github.com/johnquangdev/laverte-home
go get github.com/labstack/echo/v4@v4.15.4
go get github.com/joho/godotenv@v1.5.1
go get github.com/kelseyhightower/envconfig@v1.4.0
go get go.uber.org/zap@v1.28.0
go get github.com/google/uuid@v1.6.0
go get github.com/golang-jwt/jwt/v5@v5.3.1
go get golang.org/x/oauth2@v0.36.0
go get github.com/redis/go-redis/v9@v9.21.0
go get gorm.io/gorm@v1.25.12
go get gorm.io/driver/postgres@v1.5.11
go get github.com/jackc/pgx/v5@v5.9.2
go get github.com/rubenv/sql-migrate@v1.8.1
go get github.com/robfig/cron/v3@v3.0.1
```

- [ ] **Step 2: Write `.gitignore`**

```
/tmp/
*.env
!.env.example
```

- [ ] **Step 3: Write `config/config.go`**

```go
package config

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Port        string `envconfig:"PORT" default:"14000"`
	AppName     string `envconfig:"APP_NAME" default:"laverte-home-backend"`
	Environment string `envconfig:"ENVIRONMENT" default:"development"`

	PostgresHost     string `envconfig:"POSTGRES_HOST" default:"localhost"`
	PostgresPort     string `envconfig:"POSTGRES_PORT" default:"5432"`
	PostgresUser     string `envconfig:"POSTGRES_USER" default:"laverte"`
	PostgresPassword string `envconfig:"POSTGRES_PASSWORD" default:"laverte"`
	PostgresDB       string `envconfig:"POSTGRES_DB" default:"laverte"`

	JWTAccessSecret     string `envconfig:"JWT_ACCESS_SECRET" required:"true"`
	JWTRefreshSecret    string `envconfig:"JWT_REFRESH_SECRET" required:"true"`
	JWTAccessTTLMinutes int    `envconfig:"JWT_ACCESS_TTL_MINUTES" default:"15"`
	JWTRefreshTTLDays   int    `envconfig:"JWT_REFRESH_TTL_DAYS" default:"30"`

	GoogleClientID     string `envconfig:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `envconfig:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURI  string `envconfig:"GOOGLE_REDIRECT_URI"`

	FrontendURL string `envconfig:"FRONTEND_URL" default:"http://localhost:3000"`
	CORSOrigins string `envconfig:"CORS_ORIGINS" default:"http://localhost:3000"`

	RedisURL string `envconfig:"REDIS_URL" default:"redis://localhost:6379"`

	AdminUserIDs []uint `envconfig:"ADMIN_USER_IDS"`

	SePayAPIKey         string `envconfig:"SEPAY_API_KEY"`
	SePayBankAccount    string `envconfig:"SEPAY_BANK_ACCOUNT"`
	SePayBankCode       string `envconfig:"SEPAY_BANK_CODE"`
	SePayWebhookSecret  string `envconfig:"SEPAY_WEBHOOK_SECRET"`
	SePayTransferPrefix string `envconfig:"SEPAY_TRANSFER_PREFIX" default:"LAVERTE"`

	BookingPendingTTLMinutes       int `envconfig:"BOOKING_PENDING_TTL_MINUTES" default:"15"`
	BookingCheckinAlertLeadMinutes int `envconfig:"BOOKING_CHECKIN_ALERT_LEAD_MINUTES" default:"30"`

	GoogleCalendarCredentialsJSON string `envconfig:"GOOGLE_CALENDAR_CREDENTIALS_JSON"`
	GoogleCalendarTimeZone        string `envconfig:"GOOGLE_CALENDAR_TIMEZONE" default:"Asia/Ho_Chi_Minh"`

	ZNSAccessToken                string `envconfig:"ZNS_ACCESS_TOKEN"`
	ZNSBookingConfirmedTemplateID string `envconfig:"ZNS_BOOKING_CONFIRMED_TEMPLATE_ID"`
	ZNSLockCodeTemplateID         string `envconfig:"ZNS_LOCK_CODE_TEMPLATE_ID"`
	ZNSEndpoint                   string `envconfig:"ZNS_ENDPOINT" default:"https://business.openapi.zalo.me/message/template"`

	SMTPHost        string `envconfig:"SMTP_HOST"`
	SMTPPort        string `envconfig:"SMTP_PORT" default:"587"`
	SMTPUsername    string `envconfig:"SMTP_USERNAME"`
	SMTPPassword    string `envconfig:"SMTP_PASSWORD"`
	SMTPFrom        string `envconfig:"SMTP_FROM" default:"no-reply@laverte-home.vn"`
	AdminAlertEmail string `envconfig:"ADMIN_ALERT_EMAIL"`

	RateLimitBookingPerMinIP    int `envconfig:"RATE_LIMIT_BOOKING_PER_MIN_IP" default:"10"`
	RateLimitBookingPerMinPhone int `envconfig:"RATE_LIMIT_BOOKING_PER_MIN_PHONE" default:"5"`
	RateLimitWebhookPerMin      int `envconfig:"RATE_LIMIT_WEBHOOK_PER_MIN" default:"120"`
	RateLimitPublicPerMin       int `envconfig:"RATE_LIMIT_PUBLIC_PER_MIN" default:"20"`
	RateLimitAuthedPerMin       int `envconfig:"RATE_LIMIT_AUTHED_PER_MIN" default:"120"`
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB)
}

var cfg *Config

// GetConfig loads env once (godotenv then envconfig) and caches the result.
func GetConfig() *Config {
	if cfg != nil {
		return cfg
	}
	_ = godotenv.Load()
	cfg = &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}
```

- [ ] **Step 4: Write `config/config_test.go`**

```go
package config

import "testing"

func TestGetConfigDefaults(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	cfg = nil // reset singleton so this test controls the env it reads

	c := GetConfig()

	if c.Port != "14000" {
		t.Errorf("Port = %q, want 14000", c.Port)
	}
	if c.BookingPendingTTLMinutes != 15 {
		t.Errorf("BookingPendingTTLMinutes = %d, want 15", c.BookingPendingTTLMinutes)
	}
	if c.BookingCheckinAlertLeadMinutes != 30 {
		t.Errorf("BookingCheckinAlertLeadMinutes = %d, want 30", c.BookingCheckinAlertLeadMinutes)
	}
}

func TestDatabaseURL(t *testing.T) {
	c := &Config{PostgresHost: "h", PostgresPort: "5432", PostgresUser: "u", PostgresPassword: "p", PostgresDB: "d"}
	want := "host=h port=5432 user=u password=p dbname=d sslmode=disable"
	if got := c.DatabaseURL(); got != want {
		t.Errorf("DatabaseURL() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./config/... -v`
Expected: PASS (both `TestGetConfigDefaults` and `TestDatabaseURL`)

- [ ] **Step 6: Write `errors/errors.go`**

```go
package errors

import (
	"errors"
	"net/http"
)

type Code string

const (
	CodeInternal       Code = "INTERNAL_ERROR"
	CodeNotFound       Code = "NOT_FOUND"
	CodeSlotConflict   Code = "SLOT_CONFLICT"
	CodeBookingExpired Code = "BOOKING_EXPIRED"
	CodeAmountMismatch Code = "PAYMENT_AMOUNT_MISMATCH"
	CodeValidation     Code = "VALIDATION_ERROR"
	CodeUnauthorized   Code = "UNAUTHORIZED"
)

type Error struct {
	Code     Code
	CodeID   string
	HTTPCode int
	Message  string
	Raw      error
}

func (e *Error) Error() string {
	if e.Raw != nil {
		return e.Message + ": " + e.Raw.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Raw }

// As reports whether err (or something it wraps) is *Error.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func Internal(raw error) *Error {
	return &Error{Code: CodeInternal, CodeID: "internal_error", HTTPCode: http.StatusInternalServerError, Message: "internal error", Raw: raw}
}

func NotFound(raw error) *Error {
	return &Error{Code: CodeNotFound, CodeID: "not_found", HTTPCode: http.StatusNotFound, Message: "khong tim thay", Raw: raw}
}

func Validation(msg string) *Error {
	return &Error{Code: CodeValidation, CodeID: "validation_error", HTTPCode: http.StatusBadRequest, Message: msg}
}

func Unauthorized(raw error) *Error {
	return &Error{Code: CodeUnauthorized, CodeID: "unauthorized", HTTPCode: http.StatusUnauthorized, Message: "chua dang nhap", Raw: raw}
}

func SlotConflict(raw error) *Error {
	return &Error{Code: CodeSlotConflict, CodeID: "slot_conflict", HTTPCode: http.StatusConflict, Message: "khung gio nay vua co nguoi dat, vui long chon gio khac", Raw: raw}
}

func BookingExpired(raw error) *Error {
	return &Error{Code: CodeBookingExpired, CodeID: "booking_expired", HTTPCode: http.StatusGone, Message: "booking da het han giu cho", Raw: raw}
}

func AmountMismatch(raw error) *Error {
	return &Error{Code: CodeAmountMismatch, CodeID: "payment_amount_mismatch", HTTPCode: http.StatusBadRequest, Message: "so tien thanh toan khong khop", Raw: raw}
}
```

- [ ] **Step 7: Write `errors/errors_test.go`**

```go
package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestAsUnwrapsWrappedError(t *testing.T) {
	raw := errors.New("boom")
	wrapped := fmt.Errorf("context: %w", Internal(raw))

	e, ok := As(wrapped)
	if !ok {
		t.Fatal("As() = false, want true")
	}
	if e.Code != CodeInternal {
		t.Errorf("Code = %q, want %q", e.Code, CodeInternal)
	}
}

func TestAsFalseForPlainError(t *testing.T) {
	if _, ok := As(errors.New("plain")); ok {
		t.Error("As() = true for a plain error, want false")
	}
}

func TestErrorStringIncludesRaw(t *testing.T) {
	e := NotFound(errors.New("row missing"))
	if got := e.Error(); got != "khong tim thay: row missing" {
		t.Errorf("Error() = %q", got)
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./errors/... -v`
Expected: PASS

- [ ] **Step 9: Write `delivery/http/http.go`**

```go
package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

type Server struct {
	echo *echo.Echo
	cfg  config.Config
	log  *zap.Logger
}

// NewServer builds the Echo instance with base middleware and /health only.
// Task 6 rewrites it to take a Deps struct once there are route groups to mount.
func NewServer(cfg config.Config, log *zap.Logger) *Server {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection: "1; mode=block", ContentTypeNosniff: "nosniff",
		XFrameOptions: "DENY", ReferrerPolicy: "no-referrer",
	}))
	allowOrigins := splitOrigins(cfg.CORSOrigins)
	if len(allowOrigins) == 0 {
		allowOrigins = []string{cfg.FrontendURL}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{echo.GET, echo.HEAD, echo.POST, echo.PUT, echo.PATCH, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	return &Server{echo: e, cfg: cfg, log: log}
}

func (s *Server) Echo() *echo.Echo { return s.echo }

func (s *Server) Start() error {
	return s.echo.Start(":" + s.cfg.Port)
}

func splitOrigins(raw string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			if v := trimSpace(raw[start:i]); v != "" {
				out = append(out, v)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
```

- [ ] **Step 10: Write `delivery/http/http_test.go`**

```go
package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(config.Config{FrontendURL: "http://localhost:3000"}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}`+"\n" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestSplitOrigins(t *testing.T) {
	got := splitOrigins("http://a.com, http://b.com,,http://c.com")
	want := []string{"http://a.com", "http://b.com", "http://c.com"}
	if len(got) != len(want) {
		t.Fatalf("splitOrigins() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitOrigins()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
```

- [ ] **Step 11: Run tests to verify they pass**

Run: `go test ./delivery/http/... -v`
Expected: PASS

- [ ] **Step 12: Write `cmd/main.go`**

```go
package main

import (
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
)

func main() {
	cfg := config.GetConfig()

	log, _ := zap.NewProduction()
	defer func() { _ = log.Sync() }()

	srv := httpserver.NewServer(*cfg, log)
	log.Info("starting server", zap.String("port", cfg.Port))
	if err := srv.Start(); err != nil {
		log.Fatal("server error", zap.Error(err))
	}
}
```

- [ ] **Step 13: Write `.env.example`**

```
PORT=14000
APP_NAME=laverte-home-backend
ENVIRONMENT=development

POSTGRES_HOST=localhost
POSTGRES_PORT=55432
POSTGRES_USER=laverte
POSTGRES_PASSWORD=laverte
POSTGRES_DB=laverte

JWT_ACCESS_SECRET=
JWT_REFRESH_SECRET=
JWT_ACCESS_TTL_MINUTES=15
JWT_REFRESH_TTL_DAYS=30

GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URI=

FRONTEND_URL=http://localhost:3000
CORS_ORIGINS=http://localhost:3000

REDIS_URL=redis://localhost:6380

ADMIN_USER_IDS=

SEPAY_API_KEY=
SEPAY_BANK_ACCOUNT=
SEPAY_BANK_CODE=
SEPAY_WEBHOOK_SECRET=
SEPAY_TRANSFER_PREFIX=LAVERTE

BOOKING_PENDING_TTL_MINUTES=15
BOOKING_CHECKIN_ALERT_LEAD_MINUTES=30

GOOGLE_CALENDAR_CREDENTIALS_JSON=
GOOGLE_CALENDAR_TIMEZONE=Asia/Ho_Chi_Minh

ZNS_ACCESS_TOKEN=
ZNS_BOOKING_CONFIRMED_TEMPLATE_ID=
ZNS_LOCK_CODE_TEMPLATE_ID=
ZNS_ENDPOINT=https://business.openapi.zalo.me/message/template

SMTP_HOST=
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=no-reply@laverte-home.vn
ADMIN_ALERT_EMAIL=

RATE_LIMIT_BOOKING_PER_MIN_IP=10
RATE_LIMIT_BOOKING_PER_MIN_PHONE=5
RATE_LIMIT_WEBHOOK_PER_MIN=120
RATE_LIMIT_PUBLIC_PER_MIN=20
RATE_LIMIT_AUTHED_PER_MIN=120
```

- [ ] **Step 14: Build and commit**

```bash
go build ./...
go vet ./...
git add go.mod go.sum .gitignore .env.example config errors delivery cmd
git commit -m "feat: project skeleton with config, apperr, health-check server"
```

---

### Task 2: Postgres client, sql-migrate runner, first migration (users/refresh_tokens), Docker Compose

**Files:**
- Create: `client/postgres/client.go`
- Create: `migrations/migrations.go`
- Create: `migrations/0001_init.sql`
- Create: `cmd/migrate/main.go`
- Create: `docker-compose.dev.yml`
- Modify: `cmd/main.go` (run migrations before starting the server)
- Test: `migrations/migrations_integration_test.go`

**Interfaces:**
- Consumes: `config.GetConfig()`, `(*Config).DatabaseURL()` (Task 1).
- Produces: `postgres.GetClient(ctx context.Context) *gorm.DB`, `migrations.FS embed.FS`.

- [ ] **Step 1: Write `docker-compose.dev.yml`**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: laverte
      POSTGRES_PASSWORD: laverte
      POSTGRES_DB: laverte
    ports:
      # Host 5432/6379 are taken by another project's stack on this machine, so
      # bind laverte's stack to its own ports rather than fighting for them.
      - "55432:5432"
    volumes:
      - laverte_pg_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    ports:
      - "6380:6379"

volumes:
  laverte_pg_data:
```

- [ ] **Step 2: Start local infra**

Run: `docker compose -f docker-compose.dev.yml up -d`
Expected: `docker compose -f docker-compose.dev.yml ps` lists both containers as
`Up`, with Postgres published on host 55432 and Redis on 6380. Neither image
declares a HEALTHCHECK, so `Up` is the strongest status they ever report — do not
wait for `healthy`. Confirm Postgres really accepts connections with the
migration test in Step 8 rather than by reading the status column.

- [ ] **Step 3: Write `client/postgres/client.go`**

```go
package postgres

import (
	"context"
	"log"
	"sync"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
)

var (
	once sync.Once
	db   *gorm.DB
)

// GetClient lazily opens the shared *gorm.DB on first call.
func GetClient(ctx context.Context) *gorm.DB {
	once.Do(func() {
		cfg := config.GetConfig()
		conn, err := gorm.Open(postgres.Open(cfg.DatabaseURL()), &gorm.Config{})
		if err != nil {
			log.Fatalf("postgres: connect: %v", err)
		}
		db = conn
	})
	return db.WithContext(ctx)
}
```

- [ ] **Step 4: Write `migrations/0001_init.sql`**

```sql
-- +migrate Up
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT,
    oauth_provider TEXT NOT NULL,
    oauth_id TEXT NOT NULL,
    phone TEXT UNIQUE,
    role TEXT NOT NULL DEFAULT 'user',
    admin_granted_by BIGINT,
    admin_granted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

CREATE TABLE refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    token_id TEXT NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    family_id TEXT NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens(family_id);

-- +migrate Down
DROP TABLE refresh_tokens;
DROP TABLE users;
```

- [ ] **Step 5: Write `migrations/migrations.go`**

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 6: Write `cmd/migrate/main.go`**

```go
package main

import (
	"database/sql"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/migrations"
)

func main() {
	cfg := config.GetConfig()
	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("migrate: open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Error("migrate: close db", zap.Error(cerr))
		}
	}()

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	n, err := migrate.Exec(db, "postgres", src, migrate.Up)
	if err != nil {
		log.Fatalf("migrate: exec: %v", err)
	}
	log.Printf("migrations applied: %d", n)
}
```

- [ ] **Step 7: Write `migrations/migrations_integration_test.go`**

```go
package migrations

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
)

// requires `docker compose -f docker-compose.dev.yml up -d` and
// TEST_DATABASE_URL, e.g.
// postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable
func TestMigrationsApplyCleanly(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	}()

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: FS, Root: "."}
	if _, err := migrate.Exec(db, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if _, err := migrate.Exec(db, "postgres", src, migrate.Down); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
}
```

- [ ] **Step 8: Run the migration test against the running Postgres container**

Run: `TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test ./migrations/... -v`
Expected: PASS — the migration applies (up) and rolls back (down) cleanly. Without `TEST_DATABASE_URL` the test SKIPs, which is the intended behaviour on a machine with no local Postgres.

- [ ] **Step 9: Modify `cmd/main.go` to run migrations before starting the server**

```go
package main

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
	"github.com/johnquangdev/laverte-home/migrations"
)

func main() {
	cfg := config.GetConfig()

	log, _ := zap.NewProduction()
	defer func() { _ = log.Sync() }()

	if err := runMigrations(cfg, log); err != nil {
		log.Fatal("auto-migration failed", zap.Error(err))
	}

	srv := httpserver.NewServer(*cfg, log)
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
```

- [ ] **Step 10: Build, vet, commit**

```bash
go build ./...
go vet ./...
git add client migrations cmd docker-compose.dev.yml
git commit -m "feat: postgres client, sql-migrate runner, users/refresh_tokens schema"
```

---

### Task 3: Redis rate limiter (ported from lumen)

**Files:**
- Create: `util/ratelimit/interface.go`
- Create: `util/ratelimit/redis.go`
- Create: `util/ratelimit/memory.go`
- Create: `util/ratelimit/memory_test.go`

**Interfaces:**
- Consumes: `config.GetConfig()` (Task 1).
- Produces: `ratelimit.ILimiter` with `Allow(ctx, key string, limit int, window time.Duration) (Result, error)`; `ratelimit.NewRedis(cfg *config.Config) ILimiter`; `ratelimit.NewMemory() ILimiter` (used by tests and any dev fallback).

This is a direct, unmodified port of lumen's `util/ratelimit` package (fixed-window Redis counter + in-memory implementation for tests) — copied because it already fails open on Redis errors and has no lumen-specific dependency.

- [ ] **Step 1: Write `util/ratelimit/interface.go`**

```go
package ratelimit

import (
	"context"
	"time"
)

type Result struct {
	Allowed    bool
	RetryAfter time.Duration
}

// ILimiter is a fixed-window request counter keyed by an arbitrary string
// (per-IP or per-phone). Backed by Redis in production, in-memory for tests.
type ILimiter interface {
	// Allow records one hit against key and reports whether it stays within
	// limit requests per window.
	//
	// Result is only meaningful when err is nil — on error it is the zero
	// value, whose Allowed is false. Callers MUST branch on err before reading
	// Allowed, or a backend outage reads as "deny everything". Whether an
	// outage fails open or closed is deliberately the caller's policy: the HTTP
	// middleware fails open, so an infra blip degrades protection rather than
	// taking the API down.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
}
```

- [ ] **Step 2: Write `util/ratelimit/redis.go`**

```go
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/johnquangdev/laverte-home/config"
)

type redisLimiter struct{ client *redis.Client }

func NewRedis(cfg *config.Config) ILimiter {
	// ParseURL's error embeds the URL, which can carry a password — keep it out
	// of a panic that lands in crash logs.
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("util/ratelimit/redis: REDIS_URL is not a valid redis:// URL")
	}
	return &redisLimiter{client: redis.NewClient(opt)}
}

// Allow uses a plain pipeline on purpose — no MULTI/EXEC, no Lua script. INCR
// alone is atomic, so concurrent callers on one key each get a distinct
// strictly-increasing count, and that count alone decides the verdict. ExpireNX
// is idempotent, so racing first-hits settle on whichever TTL lands first, and
// PTTL only feeds the Retry-After hint.
func (r *redisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	pttl := pipe.PTTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return Result{}, fmt.Errorf("ratelimit: allow %q: %w", key, err)
	}

	if incr.Val() <= int64(limit) {
		return Result{Allowed: true}, nil
	}

	retryAfter := pttl.Val()
	if retryAfter <= 0 {
		retryAfter = window
	}
	return Result{Allowed: false, RetryAfter: retryAfter}, nil
}
```

- [ ] **Step 3: Write `util/ratelimit/memory.go`** (in-process fixed-window counter, used by unit tests that shouldn't need Redis)

```go
package ratelimit

import (
	"context"
	"sync"
	"time"
)

type memoryLimiter struct {
	mu     sync.Mutex
	counts map[string]*window
}

type window struct {
	count   int
	expires time.Time
}

func NewMemory() ILimiter {
	return &memoryLimiter{counts: make(map[string]*window)}
}

func (m *memoryLimiter) Allow(_ context.Context, key string, limit int, win time.Duration) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	w, ok := m.counts[key]
	if !ok || now.After(w.expires) {
		w = &window{count: 0, expires: now.Add(win)}
		m.counts[key] = w
	}
	w.count++

	if w.count <= limit {
		return Result{Allowed: true}, nil
	}
	return Result{Allowed: false, RetryAfter: w.expires.Sub(now)}, nil
}
```

- [ ] **Step 4: Write `util/ratelimit/memory_test.go`**

```go
package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLimiterAllowsUpToLimit(t *testing.T) {
	l := NewMemory()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		res, err := l.Allow(ctx, "k1", 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow() error = %v", err)
		}
		if !res.Allowed {
			t.Fatalf("Allow() call %d = denied, want allowed", i+1)
		}
	}

	res, err := l.Allow(ctx, "k1", 3, time.Minute)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if res.Allowed {
		t.Fatal("Allow() call 4 = allowed, want denied")
	}
	if res.RetryAfter <= 0 {
		t.Error("RetryAfter should be positive when denied")
	}
}

func TestMemoryLimiterKeysAreIndependent(t *testing.T) {
	l := NewMemory()
	ctx := context.Background()

	res, err := l.Allow(ctx, "k2", 1, time.Minute)
	if err != nil || !res.Allowed {
		t.Fatalf("Allow(k2) = %+v, %v", res, err)
	}
	res, err = l.Allow(ctx, "k3", 1, time.Minute)
	if err != nil || !res.Allowed {
		t.Fatalf("Allow(k3) = %+v, %v", res, err)
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./util/ratelimit/... -v`
Expected: PASS (`TestMemoryLimiterAllowsUpToLimit`, `TestMemoryLimiterKeysAreIndependent`)

- [ ] **Step 6: Commit**

```bash
git add util/ratelimit
git commit -m "feat: port Redis + in-memory rate limiter from lumen"
```

---

### Task 4: JWT util, refresh-token Redis store, Google OAuth util

**Files:**
- Create: `util/jwt.go`
- Create: `util/jwt_test.go`
- Create: `util/tokenstore/interface.go`
- Create: `util/tokenstore/redis.go`
- Create: `util/oauth/interface.go`
- Create: `util/oauth/google.go`
- Modify: `config/config.go` — none needed, fields already present from Task 1.

**Interfaces:**
- Produces: `util.Claims{UserID uint, TokenID string, FamilyID string, jwt.RegisteredClaims}`, `util.GenerateToken(secret string, claims Claims, ttl time.Duration) (string, error)`, `util.ParseToken(secret, tokenStr string) (*Claims, error)`.
- Produces: `tokenstore.ITokenStore` with `SaveState`, `ValidateState`, `BlacklistToken`, `IsBlacklisted`; `tokenstore.NewRedis(cfg *config.Config) ITokenStore`.
- Produces: `oauth.IOAuthProvider` with `AuthorizeURL(ctx) (AuthorizeURLResult, error)`, `Exchange(ctx, code, state string) (ExchangeResult, error)`; `oauth.NewGoogle(cfg *config.Config) IOAuthProvider`.

This is a direct port of lumen's `util/jwt.go`, `util/tokenstore`, `util/oauth` — unchanged logic, only the import path changes.

- [ ] **Step 1: Write `util/jwt.go`**

```go
package util

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   uint   `json:"user_id"`
	TokenID  string `json:"token_id"`
	FamilyID string `json:"family_id"`
	jwt.RegisteredClaims
}

func GenerateToken(secret string, claims Claims, ttl time.Duration) (string, error) {
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(ttl))
	claims.IssuedAt = jwt.NewNumericDate(time.Now())
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseToken(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
```

- [ ] **Step 2: Write `util/jwt_test.go`**

```go
package util

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndParseToken(t *testing.T) {
	claims := Claims{UserID: 42, TokenID: "tok-1", FamilyID: "fam-1"}
	tokenStr, err := GenerateToken("secret", claims, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	got, err := ParseToken("secret", tokenStr)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if got.UserID != 42 || got.TokenID != "tok-1" || got.FamilyID != "fam-1" {
		t.Errorf("ParseToken() = %+v", got)
	}
}

func TestParseTokenRejectsWrongSecret(t *testing.T) {
	tokenStr, _ := GenerateToken("secret-a", Claims{UserID: 1}, time.Hour)
	if _, err := ParseToken("secret-b", tokenStr); err == nil {
		t.Error("ParseToken() with wrong secret = nil error, want error")
	}
}

func TestParseTokenRejectsExpired(t *testing.T) {
	tokenStr, _ := GenerateToken("secret", Claims{UserID: 1}, -time.Minute)
	if _, err := ParseToken("secret", tokenStr); err == nil {
		t.Error("ParseToken() with expired token = nil error, want error")
	}
}

// ParseToken must reject a token whose header claims a non-HMAC algorithm.
// Without the signing-method check, an attacker could present an unsigned
// (alg=none) token and have its claims trusted.
func TestParseTokenRejectsNonHMACAlgorithm(t *testing.T) {
	claims := Claims{UserID: 99, TokenID: "forged"}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building alg=none token: %v", err)
	}

	if _, err := ParseToken("secret", unsigned); err == nil {
		t.Error("ParseToken() accepted an alg=none token, want rejection")
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test ./util/... -run TestGenerateAndParseToken -v && go test ./util/... -run TestParseToken -v`
Expected: PASS

- [ ] **Step 4: Write `util/tokenstore/interface.go`**

```go
package tokenstore

import "context"

// ITokenStore backs OAuth CSRF-state single-use checks and JWT-blacklist-on-logout.
type ITokenStore interface {
	SaveState(ctx context.Context, state string) error
	ValidateState(ctx context.Context, state string) (bool, error)
	BlacklistToken(ctx context.Context, tokenID string) error
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
}
```

- [ ] **Step 5: Write `util/tokenstore/redis.go`**

```go
package tokenstore

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/johnquangdev/laverte-home/config"
)

type redisStore struct {
	client       *redis.Client
	blacklistTTL time.Duration
}

func NewRedis(cfg *config.Config) ITokenStore {
	// The error from ParseURL embeds the URL it was given, and REDIS_URL can
	// carry a password — never put it in a panic that lands in crash logs.
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("util/tokenstore/redis: REDIS_URL is not a valid redis:// URL")
	}
	// A blacklist entry only has to outlive the token it revokes. Derived from
	// the access TTL rather than hardcoded, so raising JWT_ACCESS_TTL_MINUTES
	// can't silently leave revoked tokens usable again once the key expires.
	// The extra minute absorbs clock skew between this process and Redis.
	ttl := time.Duration(cfg.JWTAccessTTLMinutes)*time.Minute + time.Minute
	return &redisStore{client: redis.NewClient(opt), blacklistTTL: ttl}
}

func (r *redisStore) SaveState(ctx context.Context, state string) error {
	return r.client.Set(ctx, "oauth:state:"+state, "1", 10*time.Minute).Err()
}

func (r *redisStore) ValidateState(ctx context.Context, state string) (bool, error) {
	n, err := r.client.Del(ctx, "oauth:state:"+state).Result()
	return n > 0, err
}

func (r *redisStore) BlacklistToken(ctx context.Context, tokenID string) error {
	return r.client.Set(ctx, "jwt:blacklist:"+tokenID, "1", r.blacklistTTL).Err()
}

func (r *redisStore) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	n, err := r.client.Exists(ctx, "jwt:blacklist:"+tokenID).Result()
	return n > 0, err
}
```

- [ ] **Step 6: Write `util/oauth/interface.go`**

```go
package oauth

import "context"

type AuthorizeURLResult struct {
	URL   string
	State string
}

type ExchangeResult struct {
	Email    string
	OAuthID  string
	Provider string
	Avatar   string
}

// IOAuthProvider abstracts OAuth2 provider (Google).
type IOAuthProvider interface {
	AuthorizeURL(ctx context.Context) (AuthorizeURLResult, error)
	Exchange(ctx context.Context, code, state string) (ExchangeResult, error)
}
```

- [ ] **Step 7: Write `util/oauth/google.go`**

```go
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/johnquangdev/laverte-home/config"
)

type googleOAuth struct {
	oc *oauth2.Config
}

func NewGoogle(cfg *config.Config) IOAuthProvider {
	return &googleOAuth{
		oc: &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURI,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

func (g *googleOAuth) AuthorizeURL(_ context.Context) (AuthorizeURLResult, error) {
	state := uuid.New().String()
	url := g.oc.AuthCodeURL(state, oauth2.AccessTypeOnline)
	return AuthorizeURLResult{URL: url, State: state}, nil
}

type googleUserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
}

func (g *googleOAuth) Exchange(ctx context.Context, code, _ string) (ExchangeResult, error) {
	token, err := g.oc.Exchange(ctx, code)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: exchange failed: %w", err)
	}

	client := g.oc.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: userinfo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: read userinfo body: %w", err)
	}

	var info googleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return ExchangeResult{}, fmt.Errorf("oauth/google: parse userinfo: %w", err)
	}

	return ExchangeResult{Email: info.Email, OAuthID: info.Sub, Provider: "google", Avatar: info.Picture}, nil
}
```

- [ ] **Step 8: Build and commit**

```bash
go build ./...
go vet ./...
git add util
git commit -m "feat: port JWT util, Redis token store, Google OAuth from lumen"
```

---

### Task 5: User/RefreshToken model, repositories, JWT middleware

**Files:**
- Create: `model/user.go`
- Create: `model/role.go`
- Create: `repository/user/interface.go`
- Create: `repository/user/pg.go`
- Create: `repository/refreshtoken/interface.go`
- Create: `repository/refreshtoken/pg.go`
- Create: `delivery/http/middleware/jwt_auth.go`
- Create: `delivery/http/middleware/jwt_auth_test.go`
- Create: `delivery/http/middleware/require_admin.go`
- Create: `delivery/http/middleware/require_admin_test.go`
- Create: `delivery/http/middleware/rate_limit.go`
- Create: `delivery/http/middleware/rate_limit_test.go`

**Interfaces:**
- Consumes: `util.Claims`, `util.ParseToken` (Task 4); `ratelimit.ILimiter` (Task 3).
- Produces: `model.User`, `model.RefreshToken`, `model.RoleUser/RoleAdmin/RoleSuperAdmin`, `model.IsGrantableRole`, `model.HasAdminAccess`.
- Produces: `userrepo.IRepository` (`GetByOAuth`, `GetByID`, `Create`, `ListByRole`, `SetRole`), `refreshtokenrepo.IRepository` (`Create`, `GetByTokenID`, `Revoke`, `RevokeFamily`).
- Produces: `middleware.JWTAuth(cfg config.Config) echo.MiddlewareFunc`, `middleware.ClaimsFromContext(c echo.Context) *util.Claims`, `middleware.AdminRoleResolver`, `middleware.RequireAdmin`, `middleware.RequireSuperAdmin`, `middleware.RateLimitByIP`, `middleware.RateLimitByPhone`.

- [ ] **Step 1: Write `model/role.go`**

```go
package model

const (
	RoleUser       = "user"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "superadmin"
)

func IsGrantableRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}

func HasAdminAccess(role string) bool {
	return role == RoleAdmin || role == RoleSuperAdmin
}
```

- [ ] **Step 2: Write `model/user.go`**

```go
package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID             uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Email          string         `gorm:"uniqueIndex;not null" json:"email"`
	Name           *string        `json:"name"`
	OAuthProvider  string         `gorm:"column:oauth_provider;not null" json:"oauth_provider"`
	OAuthID        string         `gorm:"column:oauth_id;not null" json:"oauth_id"`
	Phone          *string        `gorm:"uniqueIndex" json:"phone"`
	Role           string         `gorm:"default:'user'" json:"role"`
	AdminGrantedBy *uint          `json:"admin_granted_by"`
	AdminGrantedAt *time.Time     `json:"admin_granted_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type RefreshToken struct {
	ID        uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	TokenID   string     `gorm:"uniqueIndex;not null" json:"token_id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	FamilyID  string     `gorm:"not null;index" json:"family_id"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
}
```

- [ ] **Step 3: Write `repository/user/interface.go`**

```go
package user

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	GetByOAuth(ctx context.Context, provider, oauthID string) (*model.User, error)
	GetByID(ctx context.Context, id uint) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
	ListByRole(ctx context.Context, role string) ([]*model.User, error)
	SetRole(ctx context.Context, userID uint, role string, grantedBy *uint, grantedAt *time.Time) error
}
```

- [ ] **Step 4: Write `repository/user/pg.go`**

```go
package user

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) GetByOAuth(ctx context.Context, provider, oauthID string) (*model.User, error) {
	var u model.User
	err := r.getDB(ctx).Where("oauth_provider = ? AND oauth_id = ?", provider, oauthID).First(&u).Error
	return &u, err
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	err := r.getDB(ctx).First(&u, id).Error
	return &u, err
}

func (r *pgRepository) Create(ctx context.Context, u *model.User) error {
	return r.getDB(ctx).Create(u).Error
}

func (r *pgRepository) ListByRole(ctx context.Context, role string) ([]*model.User, error) {
	var users []*model.User
	err := r.getDB(ctx).Where("role = ?", role).Order("admin_granted_at ASC NULLS FIRST, id ASC").Find(&users).Error
	return users, err
}

func (r *pgRepository) SetRole(ctx context.Context, userID uint, role string, grantedBy *uint, grantedAt *time.Time) error {
	return r.getDB(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"role": role, "admin_granted_by": grantedBy, "admin_granted_at": grantedAt,
	}).Error
}
```

- [ ] **Step 5: Write `repository/refreshtoken/interface.go`**

```go
package refreshtoken

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, t *model.RefreshToken) error
	GetByTokenID(ctx context.Context, tokenID string) (*model.RefreshToken, error)
	Revoke(ctx context.Context, tokenID string) error
	RevokeFamily(ctx context.Context, familyID string) error
}
```

- [ ] **Step 6: Write `repository/refreshtoken/pg.go`**

```go
package refreshtoken

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct {
	getDB func(context.Context) *gorm.DB
}

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, t *model.RefreshToken) error {
	return r.getDB(ctx).Create(t).Error
}

func (r *pgRepository) GetByTokenID(ctx context.Context, tokenID string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := r.getDB(ctx).Where("token_id = ?", tokenID).First(&t).Error
	return &t, err
}

func (r *pgRepository) Revoke(ctx context.Context, tokenID string) error {
	return r.getDB(ctx).Model(&model.RefreshToken{}).Where("token_id = ?", tokenID).
		Update("revoked_at", time.Now()).Error
}

func (r *pgRepository) RevokeFamily(ctx context.Context, familyID string) error {
	return r.getDB(ctx).Model(&model.RefreshToken{}).Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", time.Now()).Error
}
```

- [ ] **Step 7: Write `delivery/http/middleware/jwt_auth.go`**

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/util"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

type contextKey string

const ClaimsKey contextKey = "claims"

// JWTAuth authenticates a bearer access token and rejects one that Logout has
// revoked. The store lookup is what makes logout mean anything: the JWT stays
// cryptographically valid until it expires, so without consulting the blacklist
// a logged-out token would keep working for the rest of its TTL.
//
// The blacklist check fails CLOSED — a Redis error rejects the request. This is
// the opposite of the rate limiter's stance on purpose: a limiter that can't
// reach Redis should degrade protection rather than take the API down, but an
// authorization check that can't confirm a token is still valid must not let it
// through.
func JWTAuth(cfg config.Config, store tokenstore.ITokenStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := util.ParseToken(cfg.JWTAccessSecret, tokenStr)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			revoked, err := store.IsBlacklisted(c.Request().Context(), claims.TokenID)
			if err != nil || revoked {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			c.Set(string(ClaimsKey), claims)
			return next(c)
		}
	}
}

func ClaimsFromContext(c echo.Context) *util.Claims {
	v := c.Get(string(ClaimsKey))
	if v == nil {
		return nil
	}
	claims, _ := v.(*util.Claims)
	return claims
}
```

- [ ] **Step 8: Write `delivery/http/middleware/jwt_auth_test.go`**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/util"
)

// fakeTokenStore reports whichever revocation state a test needs, so the
// middleware tests never touch Redis.
type fakeTokenStore struct {
	revoked bool
	err     error
}

func (f fakeTokenStore) SaveState(context.Context, string) error             { return nil }
func (f fakeTokenStore) ValidateState(context.Context, string) (bool, error) { return true, nil }
func (f fakeTokenStore) BlacklistToken(context.Context, string) error        { return nil }
func (f fakeTokenStore) IsBlacklisted(context.Context, string) (bool, error) {
	return f.revoked, f.err
}

func TestJWTAuthRejectsMissingHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTAuth(config.Config{JWTAccessSecret: "s"}, fakeTokenStore{})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestJWTAuthAcceptsValidToken(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "s"}
	tokenStr, _ := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7}, time.Hour)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var gotUserID uint
	handler := JWTAuth(cfg, fakeTokenStore{})(func(c echo.Context) error {
		gotUserID = ClaimsFromContext(c).UserID
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUserID != 7 {
		t.Errorf("UserID = %d, want 7", gotUserID)
	}
}

// A revoked token is still cryptographically valid, so this is the only thing
// that makes Logout mean anything.
func TestJWTAuthRejectsRevokedToken(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "s"}
	tokenStr, _ := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7, TokenID: "tok-1"}, time.Hour)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	handler := JWTAuth(cfg, fakeTokenStore{revoked: true})(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("next handler ran for a revoked token")
	}
}

// Authorization must fail closed: if the store can't confirm the token is still
// valid, the request does not get through.
func TestJWTAuthRejectsWhenStoreErrors(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "s"}
	tokenStr, _ := util.GenerateToken(cfg.JWTAccessSecret, util.Claims{UserID: 7, TokenID: "tok-1"}, time.Hour)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTAuth(cfg, fakeTokenStore{err: errors.New("redis down")})(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
```

The test file needs `"context"` and `"errors"` in its import block for the fake store and the fail-closed test.

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./delivery/http/middleware/... -run TestJWTAuth -v`
Expected: PASS

- [ ] **Step 10: Write `delivery/http/middleware/require_admin.go`**

```go
package middleware

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

// AdminRoleResolver reports the admin tier a user currently holds — deliberately
// uncached so a just-revoked admin loses access immediately.
type AdminRoleResolver interface {
	UserRole(ctx context.Context, userID uint) (string, error)
}

type AdminRoleResolverFunc func(ctx context.Context, userID uint) (string, error)

func (f AdminRoleResolverFunc) UserRole(ctx context.Context, userID uint) (string, error) {
	return f(ctx, userID)
}

func RequireAdmin(cfg config.Config, resolver AdminRoleResolver) echo.MiddlewareFunc {
	superadmins := superadminSet(cfg)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := ClaimsFromContext(c)
			if claims == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			if _, ok := superadmins[claims.UserID]; ok {
				return next(c)
			}
			if resolver == nil {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "admin_required"})
			}
			role, err := resolver.UserRole(c.Request().Context(), claims.UserID)
			if err != nil || role != model.RoleAdmin {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "admin_required"})
			}
			return next(c)
		}
	}
}

func RequireSuperAdmin(cfg config.Config) echo.MiddlewareFunc {
	superadmins := superadminSet(cfg)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := ClaimsFromContext(c)
			if claims == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			if _, ok := superadmins[claims.UserID]; !ok {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "superadmin_required"})
			}
			return next(c)
		}
	}
}

func superadminSet(cfg config.Config) map[uint]struct{} {
	set := make(map[uint]struct{}, len(cfg.AdminUserIDs))
	for _, id := range cfg.AdminUserIDs {
		set[id] = struct{}{}
	}
	return set
}
```

- [ ] **Step 11: Write `delivery/http/middleware/require_admin_test.go`**

```go
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/util"
)

func withClaims(c echo.Context, userID uint) {
	c.Set(string(ClaimsKey), &util.Claims{UserID: userID})
}

func TestRequireAdminAllowsSuperadminFromEnv(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 99)

	handler := RequireAdmin(config.Config{AdminUserIDs: []uint{99}}, nil)(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRequireAdminRejectsPlainUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withClaims(c, 1)

	resolver := AdminRoleResolverFunc(func(_ context.Context, _ uint) (string, error) { return model.RoleUser, nil })
	handler := RequireAdmin(config.Config{}, resolver)(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
```

- [ ] **Step 12: Run tests to verify they pass**

Run: `go test ./delivery/http/middleware/... -run TestRequireAdmin -v`
Expected: PASS — a superadmin listed in `ADMIN_USER_IDS` gets through with no resolver at all, and a `RoleUser` gets a 403.

- [ ] **Step 13: Write `delivery/http/middleware/rate_limit.go`**

```go
package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/util/ratelimit"
)

func RateLimitByIP(name string, limiter ratelimit.ILimiter, limit int, window time.Duration) echo.MiddlewareFunc {
	return rateLimit(limiter, limit, window, func(c echo.Context) string {
		return "rl:" + name + ":ip:" + c.RealIP()
	})
}

func RateLimitByUser(name string, limiter ratelimit.ILimiter, limit int, window time.Duration) echo.MiddlewareFunc {
	return rateLimit(limiter, limit, window, func(c echo.Context) string {
		if claims := ClaimsFromContext(c); claims != nil {
			return "rl:" + name + ":user:" + strconv.FormatUint(uint64(claims.UserID), 10)
		}
		return "rl:" + name + ":ip:" + c.RealIP()
	})
}

// RateLimitByPhone keys on a form/JSON field named "customer_phone" bound
// earlier in the handler chain via c.Set("customer_phone", phone). Falls back
// to per-IP if the phone hasn't been set yet.
func RateLimitByPhone(name string, limiter ratelimit.ILimiter, limit int, window time.Duration) echo.MiddlewareFunc {
	return rateLimit(limiter, limit, window, func(c echo.Context) string {
		if phone, ok := c.Get("customer_phone").(string); ok && phone != "" {
			return "rl:" + name + ":phone:" + phone
		}
		return "rl:" + name + ":ip:" + c.RealIP()
	})
}

func rateLimit(limiter ratelimit.ILimiter, limit int, window time.Duration, keyFn func(echo.Context) string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			res, err := limiter.Allow(c.Request().Context(), keyFn(c), limit, window)
			if err != nil {
				return next(c) // fail open
			}
			if !res.Allowed {
				retrySec := max(int(math.Ceil(res.RetryAfter.Seconds())), 1)
				c.Response().Header().Set("Retry-After", strconv.Itoa(retrySec))
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			}
			return next(c)
		}
	}
}
```

- [ ] **Step 14: Write `delivery/http/middleware/rate_limit_test.go`**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/util/ratelimit"
)

func TestRateLimitByIPDeniesOverLimit(t *testing.T) {
	limiter := ratelimit.NewMemory()
	mw := RateLimitByIP("test", limiter, 1, time.Minute)
	handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1111"

	rec1 := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec1)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec2)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second call status = %d, want 429", rec2.Code)
	}
}

func TestRateLimitByPhoneFallsBackToIP(t *testing.T) {
	limiter := ratelimit.NewMemory()
	mw := RateLimitByPhone("test", limiter, 1, time.Minute)
	handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "5.6.7.8:2222"
	rec := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 15: Run tests, build, vet, commit**

```bash
go test ./... 
go build ./...
go vet ./...
git add model repository/user repository/refreshtoken delivery/http/middleware
git commit -m "feat: user/refresh_token model+repo, JWT/admin/rate-limit middleware"
```

---

### Task 6: Auth usecase, payload/presenter, HTTP routes, admin roster

**Files:**
- Create: `payload/auth.go`
- Create: `presenter/auth.go`
- Create: `usecase/auth/interface.go`
- Create: `usecase/auth/usecase.go`
- Create: `usecase/auth/usecase_test.go`
- Create: `delivery/http/auth/handler.go`
- Create: `delivery/http/auth/route.go`
- Create: `payload/admin.go`
- Create: `presenter/admin.go`
- Create: `usecase/admin/admins.go`
- Create: `usecase/admin/interface.go`
- Create: `usecase/admin/usecase.go`
- Create: `delivery/http/admin/handler.go`
- Create: `delivery/http/admin/route.go`
- Modify: `delivery/http/http.go` — wire auth + admin route groups, dependencies as constructor params
- Modify: `cmd/main.go` — construct repos/usecases/oauth/token-store/limiter and pass them in a `httpserver.Deps` literal

**Interfaces:**
- Consumes: `userrepo.IRepository`, `refreshtokenrepo.IRepository` (Task 5); `oauth.IOAuthProvider`, `tokenstore.ITokenStore` (Task 4); `util.GenerateToken/ParseToken` (Task 4).
- Produces: `authuc.IUseCase` (`LoginURL`, `Callback`, `RefreshToken`, `Logout`); `presenter.SessionResponse{AccessToken, RefreshToken, ExpiresIn, TokenType, User}`; `adminuc.IUseCase` (`ListAdmins`, `GrantAdmin`, `RevokeAdmin`).

- [ ] **Step 1: Write `payload/auth.go`**

```go
package payload

type GoogleCallbackRequest struct {
	Code  string `json:"code" validate:"required"`
	State string `json:"state" validate:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
```

- [ ] **Step 2: Write `presenter/auth.go`**

```go
package presenter

import "github.com/johnquangdev/laverte-home/model"

type GoogleLoginURLResponse struct {
	URL string `json:"url"`
}

type UserResponse struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type SessionResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
	TokenType    string       `json:"token_type"`
	User         UserResponse `json:"user"`
}

func ToUserResponse(u *model.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Role: u.Role}
}
```

- [ ] **Step 3: Write `usecase/auth/interface.go`**

```go
package auth

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	LoginURL(ctx context.Context) (*presenter.GoogleLoginURLResponse, error)
	Callback(ctx context.Context, req payload.GoogleCallbackRequest) (*presenter.SessionResponse, error)
	RefreshToken(ctx context.Context, refreshTokenStr string) (*presenter.SessionResponse, error)
	Logout(ctx context.Context, authHeader string) error
}
```

- [ ] **Step 4: Write `usecase/auth/usecase.go`**

```go
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	refreshtokenrepo "github.com/johnquangdev/laverte-home/repository/refreshtoken"
	userrepo "github.com/johnquangdev/laverte-home/repository/user"
	"github.com/johnquangdev/laverte-home/util"
	"github.com/johnquangdev/laverte-home/util/oauth"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

type UseCase struct {
	userRepo   userrepo.IRepository
	tokenRepo  refreshtokenrepo.IRepository
	oauth      oauth.IOAuthProvider
	tokenStore tokenstore.ITokenStore
	cfg        config.Config
}

func New(
	userRepo userrepo.IRepository,
	tokenRepo refreshtokenrepo.IRepository,
	oauthSvc oauth.IOAuthProvider,
	tokenStore tokenstore.ITokenStore,
	cfg config.Config,
) IUseCase {
	return &UseCase{userRepo: userRepo, tokenRepo: tokenRepo, oauth: oauthSvc, tokenStore: tokenStore, cfg: cfg}
}

func (uc *UseCase) LoginURL(ctx context.Context) (*presenter.GoogleLoginURLResponse, error) {
	result, err := uc.oauth.AuthorizeURL(ctx)
	if err != nil {
		return nil, err
	}
	if err := uc.tokenStore.SaveState(ctx, result.State); err != nil {
		return nil, err
	}
	return &presenter.GoogleLoginURLResponse{URL: result.URL}, nil
}

func (uc *UseCase) Callback(ctx context.Context, req payload.GoogleCallbackRequest) (*presenter.SessionResponse, error) {
	ok, err := uc.tokenStore.ValidateState(ctx, req.State)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("invalid or expired oauth state")
	}

	info, err := uc.oauth.Exchange(ctx, req.Code, req.State)
	if err != nil {
		return nil, err
	}

	user, err := uc.userRepo.GetByOAuth(ctx, info.Provider, info.OAuthID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		user = &model.User{Email: info.Email, OAuthProvider: info.Provider, OAuthID: info.OAuthID}
		if err := uc.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}
	}

	return uc.issueTokenPair(ctx, user, uuid.New().String())
}

func (uc *UseCase) RefreshToken(ctx context.Context, refreshTokenStr string) (*presenter.SessionResponse, error) {
	claims, err := util.ParseToken(uc.cfg.JWTRefreshSecret, refreshTokenStr)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	stored, err := uc.tokenRepo.GetByTokenID(ctx, claims.TokenID)
	if err != nil || stored.RevokedAt != nil {
		if err == nil {
			_ = uc.tokenRepo.RevokeFamily(ctx, stored.FamilyID)
		}
		return nil, errors.New("refresh token revoked or not found")
	}

	if err := uc.tokenRepo.Revoke(ctx, claims.TokenID); err != nil {
		return nil, err
	}

	user, err := uc.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	return uc.issueTokenPair(ctx, user, stored.FamilyID)
}

func (uc *UseCase) Logout(ctx context.Context, authHeader string) error {
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := util.ParseToken(uc.cfg.JWTAccessSecret, tokenStr)
	if err != nil {
		return nil
	}
	return uc.tokenStore.BlacklistToken(ctx, claims.TokenID)
}

func (uc *UseCase) issueTokenPair(ctx context.Context, user *model.User, familyID string) (*presenter.SessionResponse, error) {
	accessTokenID := uuid.New().String()
	refreshTokenID := uuid.New().String()

	accessTTL := time.Duration(uc.cfg.JWTAccessTTLMinutes) * time.Minute
	refreshTTL := time.Duration(uc.cfg.JWTRefreshTTLDays) * 24 * time.Hour

	accessToken, err := util.GenerateToken(uc.cfg.JWTAccessSecret, util.Claims{UserID: user.ID, TokenID: accessTokenID, FamilyID: familyID}, accessTTL)
	if err != nil {
		return nil, err
	}
	refreshToken, err := util.GenerateToken(uc.cfg.JWTRefreshSecret, util.Claims{UserID: user.ID, TokenID: refreshTokenID, FamilyID: familyID}, refreshTTL)
	if err != nil {
		return nil, err
	}
	if err := uc.tokenRepo.Create(ctx, &model.RefreshToken{TokenID: refreshTokenID, UserID: user.ID, FamilyID: familyID}); err != nil {
		return nil, err
	}

	return &presenter.SessionResponse{
		AccessToken: accessToken, RefreshToken: refreshToken,
		ExpiresIn: uc.cfg.JWTAccessTTLMinutes * 60, TokenType: "Bearer",
		User: presenter.ToUserResponse(user),
	}, nil
}
```

- [ ] **Step 5: Write `usecase/auth/usecase_test.go`** — exercise `RefreshToken`'s reuse-detection with fakes (no DB needed)

```go
package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/util"
	"github.com/johnquangdev/laverte-home/util/oauth"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

type fakeUserRepo struct{ users map[uint]*model.User }

func (f *fakeUserRepo) GetByOAuth(context.Context, string, string) (*model.User, error) {
	return nil, gorm.ErrRecordNotFound
}
func (f *fakeUserRepo) GetByID(_ context.Context, id uint) (*model.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return u, nil
}
func (f *fakeUserRepo) Create(_ context.Context, u *model.User) error { f.users[u.ID] = u; return nil }
func (f *fakeUserRepo) ListByRole(context.Context, string) ([]*model.User, error) { return nil, nil }
func (f *fakeUserRepo) SetRole(context.Context, uint, string, *uint, *time.Time) error { return nil }

type fakeTokenRepo struct {
	byID   map[string]*model.RefreshToken
	family map[string]bool // true once revoked
}

func newFakeTokenRepo() *fakeTokenRepo {
	return &fakeTokenRepo{byID: map[string]*model.RefreshToken{}, family: map[string]bool{}}
}
func (f *fakeTokenRepo) Create(_ context.Context, t *model.RefreshToken) error {
	f.byID[t.TokenID] = t
	return nil
}
func (f *fakeTokenRepo) GetByTokenID(_ context.Context, tokenID string) (*model.RefreshToken, error) {
	t, ok := f.byID[tokenID]
	if !ok {
		return nil, errors.New("not found")
	}
	return t, nil
}
func (f *fakeTokenRepo) Revoke(_ context.Context, tokenID string) error {
	now := time.Now()
	f.byID[tokenID].RevokedAt = &now
	return nil
}
func (f *fakeTokenRepo) RevokeFamily(_ context.Context, familyID string) error {
	f.family[familyID] = true
	for _, t := range f.byID {
		if t.FamilyID == familyID {
			now := time.Now()
			t.RevokedAt = &now
		}
	}
	return nil
}

func TestRefreshTokenReuseRevokesFamily(t *testing.T) {
	cfg := config.Config{JWTAccessSecret: "a", JWTRefreshSecret: "r", JWTAccessTTLMinutes: 15, JWTRefreshTTLDays: 30}
	userRepo := &fakeUserRepo{users: map[uint]*model.User{1: {ID: 1, Email: "a@b.com"}}}
	tokenRepo := newFakeTokenRepo()
	uc := New(userRepo, tokenRepo, nil, nil, cfg).(*UseCase)

	session, err := uc.issueTokenPair(context.Background(), userRepo.users[1], "fam-1")
	if err != nil {
		t.Fatalf("issueTokenPair() error = %v", err)
	}

	if _, err := uc.RefreshToken(context.Background(), session.RefreshToken); err != nil {
		t.Fatalf("first RefreshToken() error = %v", err)
	}

	// A replayed refresh token means the token leaked, so rejecting this one call
	// is not enough — every token in its family has to die with it.
	if _, err := uc.RefreshToken(context.Background(), session.RefreshToken); err == nil {
		t.Fatal("second RefreshToken() with reused token = nil error, want error")
	}
	if !tokenRepo.family["fam-1"] {
		t.Error("expected family fam-1 to be revoked after reuse")
	}
}

var _ = oauth.IOAuthProvider(nil)
var _ = tokenstore.ITokenStore(nil)
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./usecase/auth/... -v`
Expected: PASS — `TestRefreshTokenReuseRevokesFamily` proves a replayed refresh token is rejected AND takes its whole family down with it.

- [ ] **Step 7: Write `delivery/http/auth/handler.go`**

```go
package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc         authuc.IUseCase
	handleErr  HandleErrFunc
	handleOK   HandleOKFunc
}

func newHandler(uc authuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) loginURL(c echo.Context) error {
	resp, err := h.uc.LoginURL(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) callback(c echo.Context) error {
	var req payload.GoogleCallbackRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Callback(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) refresh(c echo.Context) error {
	var req payload.RefreshRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.RefreshToken(c.Request().Context(), req.RefreshToken)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) logout(c echo.Context) error {
	if err := h.uc.Logout(c.Request().Context(), c.Request().Header.Get("Authorization")); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
```

- [ ] **Step 8: Write `delivery/http/auth/route.go`**

```go
package auth

import (
	"github.com/labstack/echo/v4"

	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
)

func Init(g *echo.Group, uc authuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.GET("/google/login-url", h.loginURL)
	g.POST("/google/callback", h.callback)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}
```

- [ ] **Step 9: Write `payload/admin.go`**

```go
package payload

type GrantAdminRequest struct {
	UserID uint `json:"user_id" validate:"required"`
}
```

- [ ] **Step 10: Write `presenter/admin.go`**

```go
package presenter

type AdminListItemResponse struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}
```

- [ ] **Step 11: Write `usecase/admin/interface.go`**

```go
package admin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	ListAdmins(ctx context.Context) ([]presenter.AdminListItemResponse, error)
	GrantAdmin(ctx context.Context, req payload.GrantAdminRequest, grantedBy uint) error
	RevokeAdmin(ctx context.Context, userID uint) error
}
```

- [ ] **Step 12: Write `usecase/admin/admins.go`**

```go
package admin

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	userrepo "github.com/johnquangdev/laverte-home/repository/user"
)

type UseCase struct {
	userRepo userrepo.IRepository
}

func New(userRepo userrepo.IRepository) IUseCase {
	return &UseCase{userRepo: userRepo}
}

func (uc *UseCase) ListAdmins(ctx context.Context) ([]presenter.AdminListItemResponse, error) {
	users, err := uc.userRepo.ListByRole(ctx, model.RoleAdmin)
	if err != nil {
		return nil, err
	}
	out := make([]presenter.AdminListItemResponse, 0, len(users))
	for _, u := range users {
		out = append(out, presenter.AdminListItemResponse{ID: u.ID, Email: u.Email, Role: u.Role})
	}
	return out, nil
}

func (uc *UseCase) GrantAdmin(ctx context.Context, req payload.GrantAdminRequest, grantedBy uint) error {
	now := time.Now()
	return uc.userRepo.SetRole(ctx, req.UserID, model.RoleAdmin, &grantedBy, &now)
}

func (uc *UseCase) RevokeAdmin(ctx context.Context, userID uint) error {
	return uc.userRepo.SetRole(ctx, userID, model.RoleUser, nil, nil)
}
```

- [ ] **Step 13: Write `delivery/http/admin/handler.go`**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        adminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc adminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) listAdmins(c echo.Context) error {
	resp, err := h.uc.ListAdmins(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *Handler) grantAdmin(c echo.Context) error {
	var req payload.GrantAdminRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	grantedBy := middleware.ClaimsFromContext(c).UserID
	if err := h.uc.GrantAdmin(c.Request().Context(), req, grantedBy); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}

func (h *Handler) revokeAdmin(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	if err := h.uc.RevokeAdmin(c.Request().Context(), uint(id)); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
```

- [ ] **Step 14: Write `delivery/http/admin/route.go`**

```go
package admin

import (
	"github.com/labstack/echo/v4"

	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
)

// Init mounts admin-roster routes. superadminOnly must be
// middleware.RequireSuperAdmin — the group itself is already gated by
// RequireAdmin one level up in delivery/http/http.go.
func Init(g *echo.Group, uc adminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, superadminOnly echo.MiddlewareFunc) {
	h := newHandler(uc, handleErr, handleOK)
	admins := g.Group("/admins", superadminOnly)
	admins.GET("", h.listAdmins)
	admins.POST("", h.grantAdmin)
	admins.DELETE("/:id", h.revokeAdmin)
}
```

- [ ] **Step 15: Modify `delivery/http/http.go`** — replace the body of `NewServer` to wire auth/admin and centralize error handling

```go
package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	adminhttp "github.com/johnquangdev/laverte-home/delivery/http/admin"
	authhttp "github.com/johnquangdev/laverte-home/delivery/http/auth"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	apperr "github.com/johnquangdev/laverte-home/errors"
	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
	"github.com/johnquangdev/laverte-home/util/ratelimit"
	"github.com/johnquangdev/laverte-home/util/tokenstore"
)

type errBody struct {
	Code    string `json:"code"`
	CodeID  string `json:"code_id"`
	Message string `json:"message"`
}

type Server struct {
	echo *echo.Echo
	cfg  config.Config
	log  *zap.Logger
}

// Deps carries what the router needs to mount its route groups. It is a struct
// with named fields, not a positional parameter list, because nearly every
// later task adds one more dependency here — a named field can be appended
// without silently shifting the meaning of every existing call site.
type Deps struct {
	Limiter ratelimit.ILimiter
	// TokenStore is read by JWTAuth to reject tokens Logout has revoked.
	TokenStore        tokenstore.ITokenStore
	AuthUC            authuc.IUseCase
	AdminUC           adminuc.IUseCase
	AdminRoleResolver jwtmw.AdminRoleResolver
}

func NewServer(cfg config.Config, log *zap.Logger, deps Deps) *Server {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection: "1; mode=block", ContentTypeNosniff: "nosniff",
		XFrameOptions: "DENY", ReferrerPolicy: "no-referrer",
	}))
	allowOrigins := splitOrigins(cfg.CORSOrigins)
	if len(allowOrigins) == 0 {
		allowOrigins = []string{cfg.FrontendURL}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{echo.GET, echo.HEAD, echo.POST, echo.PUT, echo.PATCH, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
	}))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	handleErr := func(c echo.Context, err error) error {
		if e, ok := apperr.As(err); ok {
			log.Error(e.Error())
			return c.JSON(e.HTTPCode, errBody{Code: string(e.Code), CodeID: e.CodeID, Message: e.Message})
		}
		log.Error(err.Error())
		ie := apperr.Internal(err)
		return c.JSON(ie.HTTPCode, errBody{Code: string(ie.Code), CodeID: ie.CodeID, Message: ie.Message})
	}
	handleOK := func(c echo.Context, data any) error { return c.JSON(http.StatusOK, data) }

	api := e.Group("/api/v1")
	window := time.Minute
	ipLimit := jwtmw.RateLimitByIP("public", deps.Limiter, cfg.RateLimitPublicPerMin, window)
	userLimit := jwtmw.RateLimitByUser("authed", deps.Limiter, cfg.RateLimitAuthedPerMin, window)

	authhttp.Init(api.Group("/auth", ipLimit), deps.AuthUC, handleErr, handleOK)

	authed := api.Group("", jwtmw.JWTAuth(cfg, deps.TokenStore), userLimit)
	requireAdmin := jwtmw.RequireAdmin(cfg, deps.AdminRoleResolver)
	adminGroup := authed.Group("/admin", requireAdmin)
	adminhttp.Init(adminGroup, deps.AdminUC, handleErr, handleOK, jwtmw.RequireSuperAdmin(cfg))

	return &Server{echo: e, cfg: cfg, log: log}
}

func (s *Server) Echo() *echo.Echo { return s.echo }

func (s *Server) Start() error {
	return s.echo.Start(":" + s.cfg.Port)
}

func splitOrigins(raw string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			if v := trimSpace(raw[start:i]); v != "" {
				out = append(out, v)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
```

This replaces the whole file written in Task 1. `handleErr`/`handleOK` are defined here once and handed to every route group so the error shape stays uniform across the API, and `adminGroup` is hoisted into a variable because Tasks 7, 8, 10, 16 and 18 all mount further admin routes on it.

- [ ] **Step 16: Update `delivery/http/http_test.go`** to match the new `NewServer` signature

```go
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/presenter"
	"github.com/johnquangdev/laverte-home/util/ratelimit"
)

type stubAuthUC struct{}

func (stubAuthUC) LoginURL(context.Context) (*presenter.GoogleLoginURLResponse, error) { return nil, nil }
func (stubAuthUC) Callback(context.Context, payload.GoogleCallbackRequest) (*presenter.SessionResponse, error) {
	return nil, nil
}
func (stubAuthUC) RefreshToken(context.Context, string) (*presenter.SessionResponse, error) { return nil, nil }
func (stubAuthUC) Logout(context.Context, string) error                                     { return nil }

type stubAdminUC struct{}

func (stubAdminUC) ListAdmins(context.Context) ([]presenter.AdminListItemResponse, error) { return nil, nil }
func (stubAdminUC) GrantAdmin(context.Context, payload.GrantAdminRequest, uint) error      { return nil }
func (stubAdminUC) RevokeAdmin(context.Context, uint) error                                { return nil }

// newTestServer builds a router with stubbed dependencies. Later tasks add one
// more field to Deps each; because they are named, this helper only needs a new
// line per task rather than a re-ordered argument list.
// stubTokenStore reports nothing revoked, so router tests need no Redis.
type stubTokenStore struct{}

func (stubTokenStore) SaveState(context.Context, string) error            { return nil }
func (stubTokenStore) ValidateState(context.Context, string) (bool, error) { return true, nil }
func (stubTokenStore) BlacklistToken(context.Context, string) error       { return nil }
func (stubTokenStore) IsBlacklisted(context.Context, string) (bool, error) { return false, nil }

func newTestServer() *Server {
	return NewServer(config.Config{FrontendURL: "http://localhost:3000"}, zap.NewNop(), Deps{
		Limiter:           ratelimit.NewMemory(),
		TokenStore:        stubTokenStore{},
		AuthUC:            stubAuthUC{},
		AdminUC:           stubAdminUC{},
		AdminRoleResolver: jwtmw.AdminRoleResolverFunc(func(context.Context, uint) (string, error) { return "", nil }),
	})
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestSplitOrigins(t *testing.T) {
	got := splitOrigins("http://a.com, http://b.com,,http://c.com")
	want := []string{"http://a.com", "http://b.com", "http://c.com"}
	if len(got) != len(want) {
		t.Fatalf("splitOrigins() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitOrigins()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
```

Add `"github.com/johnquangdev/laverte-home/payload"` to this test file's imports.

- [ ] **Step 17: Modify `cmd/main.go`** — wire real dependencies

```go
package main

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/client/postgres"
	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
	jwtmw "github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/migrations"
	refreshtokenrepo "github.com/johnquangdev/laverte-home/repository/refreshtoken"
	userrepo "github.com/johnquangdev/laverte-home/repository/user"
	adminuc "github.com/johnquangdev/laverte-home/usecase/admin"
	authuc "github.com/johnquangdev/laverte-home/usecase/auth"
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

	oauthSvc := oauth.NewGoogle(cfg)
	tokenStore := tokenstore.NewRedis(cfg)
	limiter := ratelimit.NewRedis(cfg)

	authUC := authuc.New(users, tokens, oauthSvc, tokenStore, *cfg)
	adminUC := adminuc.New(users)

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
```

`postgres.GetClient` already has the `func(context.Context) *gorm.DB` shape every repository constructor expects, so it is passed directly as the DB factory.

- [ ] **Step 18: Build, vet, run full test suite, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add payload presenter usecase/auth usecase/admin delivery/http cmd/main.go
git commit -m "feat: auth usecase (Google OAuth+JWT), admin roster, wire server"
```

---

## Phase 1 — Booking core domain

### Task 7: Home model, migration, repository, admin CRUD

**Files:**
- Create: `model/home.go`
- Create: `migrations/0002_homes.sql`
- Create: `repository/home/interface.go`
- Create: `repository/home/pg.go`
- Create: `payload/home.go`
- Create: `presenter/home.go`
- Create: `usecase/homeadmin/interface.go`
- Create: `usecase/homeadmin/usecase.go`
- Create: `usecase/homeadmin/usecase_test.go`
- Create: `delivery/http/admin/home_handler.go`
- Create: `delivery/http/admin/home_route.go`
- Modify: `delivery/http/admin/route.go` — mount home routes
- Modify: `delivery/http/http.go`, `cmd/main.go` — wire `usecase/homeadmin`

**Interfaces:**
- Produces: `model.Home{ID, Name, Category, Address, Description, GoogleCalendarID, IsActive, CreatedAt, UpdatedAt}`, `model.HomeCategoryHome = "home"`, `model.HomeCategoryNest = "nest"`.
- Produces: `homerepo.IRepository` (`Create`, `Update`, `GetByID`, `List`).
- Produces: `homeadminuc.IUseCase` (`Create`, `Update`, `List`).

- [ ] **Step 1: Write `model/home.go`**

```go
package model

import "time"

const (
	HomeCategoryHome = "home"
	HomeCategoryNest = "nest"
)

type Home struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name             string    `gorm:"not null" json:"name"`
	Category         string    `gorm:"not null" json:"category"`
	Address          string    `json:"address"`
	Description      string    `json:"description"`
	GoogleCalendarID string    `gorm:"column:google_calendar_id" json:"google_calendar_id"`
	IsActive         bool      `gorm:"default:true" json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Home) TableName() string { return "homes" }

func IsValidHomeCategory(c string) bool {
	return c == HomeCategoryHome || c == HomeCategoryNest
}
```

- [ ] **Step 2: Write `migrations/0002_homes.sql`**

```sql
-- +migrate Up
CREATE TABLE homes (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    google_calendar_id TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_homes_category ON homes(category);

-- +migrate Down
DROP TABLE homes;
```

- [ ] **Step 3: Write `repository/home/interface.go`**

```go
package home

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, h *model.Home) error
	Update(ctx context.Context, h *model.Home) error
	GetByID(ctx context.Context, id uint) (*model.Home, error)
	List(ctx context.Context) ([]*model.Home, error)
}
```

- [ ] **Step 4: Write `repository/home/pg.go`**

```go
package home

import (
	"context"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct{ getDB func(context.Context) *gorm.DB }

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, h *model.Home) error {
	return r.getDB(ctx).Create(h).Error
}

func (r *pgRepository) Update(ctx context.Context, h *model.Home) error {
	return r.getDB(ctx).Save(h).Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.Home, error) {
	var h model.Home
	err := r.getDB(ctx).First(&h, id).Error
	return &h, err
}

func (r *pgRepository) List(ctx context.Context) ([]*model.Home, error) {
	var homes []*model.Home
	err := r.getDB(ctx).Order("id ASC").Find(&homes).Error
	return homes, err
}
```

- [ ] **Step 5: Write `payload/home.go`**

```go
package payload

type CreateHomeRequest struct {
	Name        string `json:"name" validate:"required"`
	Category    string `json:"category" validate:"required,oneof=home nest"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

type UpdateHomeRequest struct {
	Name             string `json:"name" validate:"required"`
	Category         string `json:"category" validate:"required,oneof=home nest"`
	Address          string `json:"address"`
	Description      string `json:"description"`
	GoogleCalendarID string `json:"google_calendar_id"`
	IsActive         bool   `json:"is_active"`
}
```

- [ ] **Step 6: Write `presenter/home.go`**

```go
package presenter

import "github.com/johnquangdev/laverte-home/model"

type HomeResponse struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Address          string `json:"address"`
	Description      string `json:"description"`
	GoogleCalendarID string `json:"google_calendar_id"`
	IsActive         bool   `json:"is_active"`
}

func ToHomeResponse(h *model.Home) HomeResponse {
	return HomeResponse{
		ID: h.ID, Name: h.Name, Category: h.Category, Address: h.Address,
		Description: h.Description, GoogleCalendarID: h.GoogleCalendarID, IsActive: h.IsActive,
	}
}
```

- [ ] **Step 7: Write `usecase/homeadmin/interface.go`**

```go
package homeadmin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Create(ctx context.Context, req payload.CreateHomeRequest) (*presenter.HomeResponse, error)
	Update(ctx context.Context, id uint, req payload.UpdateHomeRequest) (*presenter.HomeResponse, error)
	List(ctx context.Context) ([]presenter.HomeResponse, error)
}
```

- [ ] **Step 8: Write `usecase/homeadmin/usecase.go`**

```go
package homeadmin

import (
	"context"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
)

type UseCase struct{ repo homerepo.IRepository }

func New(repo homerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Create(ctx context.Context, req payload.CreateHomeRequest) (*presenter.HomeResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category phai la 'home' hoac 'nest'")
	}
	h := &model.Home{Name: req.Name, Category: req.Category, Address: req.Address, Description: req.Description, IsActive: true}
	if err := uc.repo.Create(ctx, h); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToHomeResponse(h)
	return &resp, nil
}

func (uc *UseCase) Update(ctx context.Context, id uint, req payload.UpdateHomeRequest) (*presenter.HomeResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category phai la 'home' hoac 'nest'")
	}
	h, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperr.NotFound(err)
	}
	h.Name, h.Category, h.Address, h.Description = req.Name, req.Category, req.Address, req.Description
	h.GoogleCalendarID, h.IsActive = req.GoogleCalendarID, req.IsActive
	if err := uc.repo.Update(ctx, h); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToHomeResponse(h)
	return &resp, nil
}

func (uc *UseCase) List(ctx context.Context) ([]presenter.HomeResponse, error) {
	homes, err := uc.repo.List(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.HomeResponse, 0, len(homes))
	for _, h := range homes {
		out = append(out, presenter.ToHomeResponse(h))
	}
	return out, nil
}
```

- [ ] **Step 9: Write `usecase/homeadmin/usecase_test.go`**

```go
package homeadmin

import (
	"context"
	"testing"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
)

type fakeRepo struct {
	homes  map[uint]*model.Home
	nextID uint
}

func newFakeRepo() *fakeRepo { return &fakeRepo{homes: map[uint]*model.Home{}} }

func (f *fakeRepo) Create(_ context.Context, h *model.Home) error {
	f.nextID++
	h.ID = f.nextID
	f.homes[h.ID] = h
	return nil
}
func (f *fakeRepo) Update(_ context.Context, h *model.Home) error { f.homes[h.ID] = h; return nil }
func (f *fakeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	h, ok := f.homes[id]
	if !ok {
		return nil, apperr.NotFound(nil)
	}
	return h, nil
}
func (f *fakeRepo) List(_ context.Context) ([]*model.Home, error) {
	out := make([]*model.Home, 0, len(f.homes))
	for _, h := range f.homes {
		out = append(out, h)
	}
	return out, nil
}

func TestCreateRejectsInvalidCategory(t *testing.T) {
	uc := New(newFakeRepo())
	_, err := uc.Create(context.Background(), payload.CreateHomeRequest{Name: "A", Category: "villa"})
	if err == nil {
		t.Fatal("Create() with invalid category = nil error, want error")
	}
}

func TestCreateAndListRoundTrip(t *testing.T) {
	uc := New(newFakeRepo())
	created, err := uc.Create(context.Background(), payload.CreateHomeRequest{Name: "Nest 1", Category: model.HomeCategoryNest})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	list, err := uc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Errorf("List() = %+v, want one item with ID %d", list, created.ID)
	}
}
```

- [ ] **Step 10: Run tests to verify they pass**

Run: `go test ./usecase/homeadmin/... -v`
Expected: PASS

- [ ] **Step 11: Write `delivery/http/admin/home_handler.go`**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
)

type HomeHandler struct {
	uc        homeadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHomeHandler(uc homeadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *HomeHandler {
	return &HomeHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *HomeHandler) create(c echo.Context) error {
	var req payload.CreateHomeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Create(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *HomeHandler) update(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.UpdateHomeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Update(c.Request().Context(), uint(id), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *HomeHandler) list(c echo.Context) error {
	resp, err := h.uc.List(c.Request().Context())
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
```

- [ ] **Step 12: Write `delivery/http/admin/home_route.go`**

```go
package admin

import (
	"github.com/labstack/echo/v4"

	homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"
)

func InitHomes(g *echo.Group, uc homeadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHomeHandler(uc, handleErr, handleOK)
	homes := g.Group("/homes")
	homes.GET("", h.list)
	homes.POST("", h.create)
	homes.PUT("/:id", h.update)
}
```

- [ ] **Step 13: Modify `delivery/http/http.go`** — add one field to `Deps` and one mount

Add the import `homeadminuc "github.com/johnquangdev/laverte-home/usecase/homeadmin"`, add this field to the `Deps` struct:

```go
	HomeAdminUC homeadminuc.IUseCase
```

and mount it on the shared `adminGroup` immediately after the existing `adminhttp.Init(...)` line:

```go
	adminhttp.InitHomes(adminGroup, deps.HomeAdminUC, handleErr, handleOK)
```

- [ ] **Step 14: Modify `delivery/http/http_test.go`** — stub the new dependency

```go
type stubHomeAdminUC struct{}

func (stubHomeAdminUC) Create(context.Context, payload.CreateHomeRequest) (*presenter.HomeResponse, error) {
	return nil, nil
}
func (stubHomeAdminUC) Update(context.Context, uint, payload.UpdateHomeRequest) (*presenter.HomeResponse, error) {
	return nil, nil
}
func (stubHomeAdminUC) List(context.Context) ([]presenter.HomeResponse, error) { return nil, nil }
```

and add `HomeAdminUC: stubHomeAdminUC{},` to the `Deps` literal in `newTestServer()`.

- [ ] **Step 15: Modify `cmd/main.go`** — wire the repo and usecase

Add next to the existing repository constructors:

```go
	homes := homerepo.NewPG(dbFactory)
	homeAdminUC := homeadminuc.New(homes)
```

and add `HomeAdminUC: homeAdminUC,` to the `httpserver.Deps{...}` literal. Add the `homerepo` and `homeadminuc` imports.

- [ ] **Step 16: Build, vet, test, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add model/home.go migrations/0002_homes.sql repository/home payload/home.go presenter/home.go usecase/homeadmin delivery/http cmd/main.go
git commit -m "feat: Home model + admin CRUD"
```

---

### Task 8: PricingRule model, migration, repository, pricing usecase (core pricing logic)

**Files:**
- Create: `model/pricing_rule.go`
- Create: `migrations/0003_pricing_rules.sql`
- Create: `repository/pricingrule/interface.go`
- Create: `repository/pricingrule/pg.go`
- Create: `usecase/pricing/interface.go`
- Create: `usecase/pricing/usecase.go`
- Create: `usecase/pricing/usecase_test.go`
- Create: `payload/pricing_rule.go`
- Create: `presenter/pricing_rule.go`
- Create: `usecase/pricingadmin/interface.go`
- Create: `usecase/pricingadmin/usecase.go`
- Create: `usecase/pricingadmin/usecase_test.go`
- Create: `delivery/http/admin/pricing_rule_handler.go`
- Create: `delivery/http/admin/pricing_rule_route.go`
- Modify: `delivery/http/http.go`, `cmd/main.go` — wire pricing admin usecase

**Interfaces:**
- Produces: `model.PricingRule{ID, Category, RuleType, BaseHours, BasePrice, ExtraHourPrice, WindowStart, WindowEnd, FlatPrice, EffectiveFrom, EffectiveTo, IsActive, CreatedAt}`, `model.PricingRuleTypeHourly/Overnight/Day`.
- Produces: `pricingrulerepo.IRepository` (`Create`, `Update`, `ListActiveByCategory`).
- Produces: `pricinguc.IUseCase` with `Compute(ctx, category, bookingType string, start, end, at time.Time) (int64, error)` — the function the booking usecase calls in Task 12.
- Produces: `pricingadminuc.IUseCase` (`Create`, `Supersede`, `ListByCategory`). `Supersede` closes a rule and inserts its replacement rather than editing in place, so `Compute` can still resolve the price a past booking was charged under.

- [ ] **Step 1: Write `model/pricing_rule.go`**

```go
package model

import "time"

const (
	PricingRuleTypeHourly    = "hourly"
	PricingRuleTypeOvernight = "overnight"
	PricingRuleTypeDay       = "day"
)

// PricingRule defines one price rule for a Home category. WindowStart/End are
// "HH:MM" strings used only by overnight rules to decide whether a booking's
// start time falls inside the overnight window (e.g. "22:00"-"06:00" wraps
// past midnight).
type PricingRule struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Category       string     `gorm:"not null;index" json:"category"`
	RuleType       string     `gorm:"not null" json:"rule_type"`
	BaseHours      *int       `json:"base_hours"`
	BasePrice      *int64     `json:"base_price"`
	ExtraHourPrice *int64     `json:"extra_hour_price"`
	WindowStart    *string    `json:"window_start"`
	WindowEnd      *string    `json:"window_end"`
	FlatPrice      *int64     `json:"flat_price"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveTo    *time.Time `json:"effective_to"`
	IsActive       bool       `gorm:"default:true" json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (PricingRule) TableName() string { return "pricing_rules" }
```

- [ ] **Step 2: Write `migrations/0003_pricing_rules.sql`**

```sql
-- +migrate Up
CREATE TABLE pricing_rules (
    id BIGSERIAL PRIMARY KEY,
    category TEXT NOT NULL,
    rule_type TEXT NOT NULL,
    base_hours INT,
    base_price BIGINT,
    extra_hour_price BIGINT,
    window_start TEXT,
    window_end TEXT,
    flat_price BIGINT,
    effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    effective_to TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pricing_rules_category_type ON pricing_rules(category, rule_type);

-- +migrate Down
DROP TABLE pricing_rules;
```

- [ ] **Step 3: Write `repository/pricingrule/interface.go`**

```go
package pricingrule

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, r *model.PricingRule) error
	Update(ctx context.Context, r *model.PricingRule) error
	GetByID(ctx context.Context, id uint) (*model.PricingRule, error)
	// ListActiveByCategory returns is_active rules for category whose
	// [EffectiveFrom, EffectiveTo) window contains at.
	ListActiveByCategory(ctx context.Context, category string, at time.Time) ([]*model.PricingRule, error)
}
```

- [ ] **Step 4: Write `repository/pricingrule/pg.go`**

```go
package pricingrule

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct{ getDB func(context.Context) *gorm.DB }

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, rule *model.PricingRule) error {
	return r.getDB(ctx).Create(rule).Error
}

func (r *pgRepository) Update(ctx context.Context, rule *model.PricingRule) error {
	return r.getDB(ctx).Save(rule).Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.PricingRule, error) {
	var rule model.PricingRule
	err := r.getDB(ctx).First(&rule, id).Error
	return &rule, err
}

func (r *pgRepository) ListActiveByCategory(ctx context.Context, category string, at time.Time) ([]*model.PricingRule, error) {
	var rules []*model.PricingRule
	err := r.getDB(ctx).
		Where("category = ? AND is_active = true AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", category, at, at).
		Find(&rules).Error
	return rules, err
}
```

- [ ] **Step 5: Write `usecase/pricing/interface.go`**

```go
package pricing

import (
	"context"
	"time"
)

type IUseCase interface {
	// Compute returns the price in VND for a booking of bookingType in
	// category, spanning [start, end), evaluated against rules effective at
	// `at` (normally start).
	Compute(ctx context.Context, category, bookingType string, start, end, at time.Time) (int64, error)
}
```

- [ ] **Step 6: Write `usecase/pricing/usecase.go`**

```go
package pricing

import (
	"context"
	"fmt"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	pricingrulerepo "github.com/johnquangdev/laverte-home/repository/pricingrule"
)

type UseCase struct{ repo pricingrulerepo.IRepository }

func New(repo pricingrulerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Compute(ctx context.Context, category, bookingType string, start, end, at time.Time) (int64, error) {
	rules, err := uc.repo.ListActiveByCategory(ctx, category, at)
	if err != nil {
		return 0, apperr.Internal(err)
	}

	var rule *model.PricingRule
	for _, r := range rules {
		if r.RuleType == bookingType {
			rule = r
			break
		}
	}
	if rule == nil {
		return 0, apperr.Validation(fmt.Sprintf("khong tim thay bang gia cho hang %q, loai %q", category, bookingType))
	}

	switch bookingType {
	case model.PricingRuleTypeHourly:
		return computeHourly(rule, start, end)
	case model.PricingRuleTypeOvernight, model.PricingRuleTypeDay:
		return computeFlat(rule, start)
	default:
		return 0, apperr.Validation(fmt.Sprintf("booking_type khong hop le: %q", bookingType))
	}
}

func computeHourly(rule *model.PricingRule, start, end time.Time) (int64, error) {
	if rule.BaseHours == nil || rule.BasePrice == nil || rule.ExtraHourPrice == nil {
		return 0, apperr.Validation("bang gia hourly thieu base_hours/base_price/extra_hour_price")
	}
	durationHours := end.Sub(start).Hours()
	if durationHours <= 0 {
		return 0, apperr.Validation("end_time phai sau start_time")
	}
	if durationHours <= float64(*rule.BaseHours) {
		return *rule.BasePrice, nil
	}
	extraHours := durationHours - float64(*rule.BaseHours)
	extraWhole := int64(extraHours)
	if extraHours > float64(extraWhole) {
		extraWhole++ // round any partial extra hour up to a full extra-hour charge
	}
	return *rule.BasePrice + extraWhole**rule.ExtraHourPrice, nil
}

func computeFlat(rule *model.PricingRule, start time.Time) (int64, error) {
	if rule.FlatPrice == nil {
		return 0, apperr.Validation("bang gia thieu flat_price")
	}
	if rule.WindowStart != nil && rule.WindowEnd != nil {
		if !withinWindow(start, *rule.WindowStart, *rule.WindowEnd) {
			return 0, apperr.Validation("start_time khong nam trong khung gio ap dung cua rule nay")
		}
	}
	return *rule.FlatPrice, nil
}

// withinWindow reports whether t's local HH:MM falls in [windowStart,
// windowEnd), wrapping past midnight when windowEnd <= windowStart (e.g.
// 22:00-06:00).
func withinWindow(t time.Time, windowStart, windowEnd string) bool {
	minutesOfDay := t.Hour()*60 + t.Minute()
	startMin := hhmmToMinutes(windowStart)
	endMin := hhmmToMinutes(windowEnd)
	if startMin <= endMin {
		return minutesOfDay >= startMin && minutesOfDay < endMin
	}
	return minutesOfDay >= startMin || minutesOfDay < endMin
}

func hhmmToMinutes(hhmm string) int {
	var h, m int
	fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	return h*60 + m
}
```

- [ ] **Step 7: Write `usecase/pricing/usecase_test.go`**

```go
package pricing

import (
	"context"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type fakePricingRuleRepo struct{ rules []*model.PricingRule }

func (f *fakePricingRuleRepo) Create(context.Context, *model.PricingRule) error { return nil }
func (f *fakePricingRuleRepo) Update(context.Context, *model.PricingRule) error { return nil }
func (f *fakePricingRuleRepo) GetByID(context.Context, uint) (*model.PricingRule, error) {
	return nil, nil
}
func (f *fakePricingRuleRepo) ListActiveByCategory(_ context.Context, category string, _ time.Time) ([]*model.PricingRule, error) {
	var out []*model.PricingRule
	for _, r := range f.rules {
		if r.Category == category {
			out = append(out, r)
		}
	}
	return out, nil
}

func int64p(v int64) *int64 { return &v }
func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }

func TestComputeHourlyWithinBaseHours(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(200000), ExtraHourPrice: int64p(50000),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Minute)
	price, err := uc.Compute(context.Background(), model.HomeCategoryHome, model.PricingRuleTypeHourly, start, end, start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if price != 200000 {
		t.Errorf("price = %d, want 200000", price)
	}
}

func TestComputeHourlyChargesRoundedExtraHours(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(200000), ExtraHourPrice: int64p(50000),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(3*time.Hour + 20*time.Minute) // 1h20m extra -> rounds up to 2 extra hours
	price, err := uc.Compute(context.Background(), model.HomeCategoryHome, model.PricingRuleTypeHourly, start, end, start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if price != 200000+2*50000 {
		t.Errorf("price = %d, want %d", price, 200000+2*50000)
	}
}

func TestComputeOvernightWithinWindow(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000), WindowStart: strp("22:00"), WindowEnd: strp("06:00"),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 23, 30, 0, 0, time.UTC) // inside 22:00-06:00 wrap
	end := start.Add(8 * time.Hour)
	price, err := uc.Compute(context.Background(), model.HomeCategoryNest, model.PricingRuleTypeOvernight, start, end, start)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if price != 500000 {
		t.Errorf("price = %d, want 500000", price)
	}
}

func TestComputeOvernightOutsideWindowRejected(t *testing.T) {
	repo := &fakePricingRuleRepo{rules: []*model.PricingRule{{
		Category: model.HomeCategoryNest, RuleType: model.PricingRuleTypeOvernight,
		FlatPrice: int64p(500000), WindowStart: strp("22:00"), WindowEnd: strp("06:00"),
	}}}
	uc := New(repo)
	start := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC) // 14:00 is outside 22:00-06:00
	end := start.Add(8 * time.Hour)
	if _, err := uc.Compute(context.Background(), model.HomeCategoryNest, model.PricingRuleTypeOvernight, start, end, start); err == nil {
		t.Fatal("Compute() outside window = nil error, want error")
	}
}

func TestComputeNoMatchingRule(t *testing.T) {
	uc := New(&fakePricingRuleRepo{})
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	if _, err := uc.Compute(context.Background(), model.HomeCategoryHome, model.PricingRuleTypeDay, start, start.Add(time.Hour), start); err == nil {
		t.Fatal("Compute() with no rules = nil error, want error")
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./usecase/pricing/... -v`
Expected: PASS on all five — base-hours-only, rounded extra hours, overnight inside the wrapping window, overnight outside it, and no-matching-rule.

- [ ] **Step 9: Write `payload/pricing_rule.go`**

```go
package payload

type UpsertPricingRuleRequest struct {
	Category       string  `json:"category" validate:"required,oneof=home nest"`
	RuleType       string  `json:"rule_type" validate:"required,oneof=hourly overnight day"`
	BaseHours      *int    `json:"base_hours"`
	BasePrice      *int64  `json:"base_price"`
	ExtraHourPrice *int64  `json:"extra_hour_price"`
	WindowStart    *string `json:"window_start"`
	WindowEnd      *string `json:"window_end"`
	FlatPrice      *int64  `json:"flat_price"`
}
```

- [ ] **Step 10: Write `presenter/pricing_rule.go`**

```go
package presenter

import "github.com/johnquangdev/laverte-home/model"

type PricingRuleResponse struct {
	ID             uint    `json:"id"`
	Category       string  `json:"category"`
	RuleType       string  `json:"rule_type"`
	BaseHours      *int    `json:"base_hours"`
	BasePrice      *int64  `json:"base_price"`
	ExtraHourPrice *int64  `json:"extra_hour_price"`
	WindowStart    *string `json:"window_start"`
	WindowEnd      *string `json:"window_end"`
	FlatPrice      *int64  `json:"flat_price"`
	IsActive       bool    `json:"is_active"`
}

func ToPricingRuleResponse(r *model.PricingRule) PricingRuleResponse {
	return PricingRuleResponse{
		ID: r.ID, Category: r.Category, RuleType: r.RuleType, BaseHours: r.BaseHours,
		BasePrice: r.BasePrice, ExtraHourPrice: r.ExtraHourPrice, WindowStart: r.WindowStart,
		WindowEnd: r.WindowEnd, FlatPrice: r.FlatPrice, IsActive: r.IsActive,
	}
}
```

- [ ] **Step 11: Write `usecase/pricingadmin/interface.go`**

```go
package pricingadmin

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Create(ctx context.Context, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error)
	// Supersede closes rule `id` and inserts req as its replacement, so a price
	// change never rewrites the rule a past booking was charged under.
	Supersede(ctx context.Context, id uint, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error)
	ListByCategory(ctx context.Context, category string) ([]presenter.PricingRuleResponse, error)
}
```

- [ ] **Step 12: Write `usecase/pricingadmin/usecase.go`**

```go
package pricingadmin

import (
	"context"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	pricingrulerepo "github.com/johnquangdev/laverte-home/repository/pricingrule"
)

type UseCase struct{ repo pricingrulerepo.IRepository }

func New(repo pricingrulerepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Create(ctx context.Context, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category khong hop le")
	}
	r := &model.PricingRule{
		Category: req.Category, RuleType: req.RuleType, BaseHours: req.BaseHours, BasePrice: req.BasePrice,
		ExtraHourPrice: req.ExtraHourPrice, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		FlatPrice: req.FlatPrice, EffectiveFrom: time.Now(), IsActive: true,
	}
	if err := uc.repo.Create(ctx, r); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToPricingRuleResponse(r)
	return &resp, nil
}

// Supersede is how a price is changed: the old rule is closed at `now` rather
// than edited, because usecase/pricing resolves rules by the booking's own
// timestamp — mutating a live rule in place would retroactively change what
// already-created bookings were priced under.
func (uc *UseCase) Supersede(ctx context.Context, id uint, req payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	if !model.IsValidHomeCategory(req.Category) {
		return nil, apperr.Validation("category khong hop le")
	}
	old, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, apperr.NotFound(err)
	}

	now := time.Now()
	old.EffectiveTo = &now
	old.IsActive = false
	if err := uc.repo.Update(ctx, old); err != nil {
		return nil, apperr.Internal(err)
	}

	replacement := &model.PricingRule{
		Category: req.Category, RuleType: req.RuleType, BaseHours: req.BaseHours, BasePrice: req.BasePrice,
		ExtraHourPrice: req.ExtraHourPrice, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		FlatPrice: req.FlatPrice, EffectiveFrom: now, IsActive: true,
	}
	if err := uc.repo.Create(ctx, replacement); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToPricingRuleResponse(replacement)
	return &resp, nil
}

func (uc *UseCase) ListByCategory(ctx context.Context, category string) ([]presenter.PricingRuleResponse, error) {
	rules, err := uc.repo.ListActiveByCategory(ctx, category, time.Now())
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.PricingRuleResponse, 0, len(rules))
	for _, r := range rules {
		out = append(out, presenter.ToPricingRuleResponse(r))
	}
	return out, nil
}
```

- [ ] **Step 13: Write `usecase/pricingadmin/usecase_test.go`**

```go
package pricingadmin

import (
	"context"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
)

type fakeRepo struct {
	byID    map[uint]*model.PricingRule
	created []*model.PricingRule
	nextID  uint
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[uint]*model.PricingRule{}}
}

func (f *fakeRepo) Create(_ context.Context, r *model.PricingRule) error {
	f.nextID++
	r.ID = f.nextID
	f.byID[r.ID] = r
	f.created = append(f.created, r)
	return nil
}

func (f *fakeRepo) Update(_ context.Context, r *model.PricingRule) error {
	f.byID[r.ID] = r
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uint) (*model.PricingRule, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return r, nil
}

func (f *fakeRepo) ListActiveByCategory(_ context.Context, category string, _ time.Time) ([]*model.PricingRule, error) {
	var out []*model.PricingRule
	for _, r := range f.byID {
		if r.Category == category && r.IsActive {
			out = append(out, r)
		}
	}
	return out, nil
}

func int64p(v int64) *int64 { return &v }
func intp(v int) *int       { return &v }

func hourlyReq(base int64) payload.UpsertPricingRuleRequest {
	return payload.UpsertPricingRuleRequest{
		Category: model.HomeCategoryHome, RuleType: model.PricingRuleTypeHourly,
		BaseHours: intp(2), BasePrice: int64p(base), ExtraHourPrice: int64p(50000),
	}
}

func TestSupersedeClosesOldRuleAndCreatesReplacement(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	original, err := uc.Create(ctx, hourlyReq(200000))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	replacement, err := uc.Supersede(ctx, original.ID, hourlyReq(250000))
	if err != nil {
		t.Fatalf("Supersede() error = %v", err)
	}

	if replacement.ID == original.ID {
		t.Error("Supersede() reused the old row; a replacement must be a new rule")
	}
	if *replacement.BasePrice != 250000 {
		t.Errorf("replacement BasePrice = %d, want 250000", *replacement.BasePrice)
	}

	// The old rule must survive as history, closed rather than rewritten — the
	// price a past booking was charged under has to stay reconstructible.
	old := repo.byID[original.ID]
	if old.IsActive {
		t.Error("old rule still active after Supersede")
	}
	if old.EffectiveTo == nil {
		t.Error("old rule EffectiveTo not set after Supersede")
	}
	if *old.BasePrice != 200000 {
		t.Errorf("old rule BasePrice = %d, want it left at 200000", *old.BasePrice)
	}
}

func TestSupersedeUnknownRuleRejected(t *testing.T) {
	uc := New(newFakeRepo())
	if _, err := uc.Supersede(context.Background(), 999, hourlyReq(250000)); err == nil {
		t.Fatal("Supersede() on a missing rule = nil error, want error")
	}
}

func TestSupersedeRejectsInvalidCategory(t *testing.T) {
	repo := newFakeRepo()
	uc := New(repo)
	ctx := context.Background()

	original, err := uc.Create(ctx, hourlyReq(200000))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	bad := hourlyReq(250000)
	bad.Category = "villa"
	if _, err := uc.Supersede(ctx, original.ID, bad); err == nil {
		t.Fatal("Supersede() with invalid category = nil error, want error")
	}
	if !repo.byID[original.ID].IsActive {
		t.Error("a rejected Supersede must not have closed the existing rule")
	}
}
```

Add `"gorm.io/gorm"` to this file's import block — `GetByID` returns `gorm.ErrRecordNotFound` so the fake matches what the pg repository really returns.

- [ ] **Step 14: Run tests to verify they pass**

Run: `go test ./usecase/pricingadmin/... -v`
Expected: PASS — the replacement is a new row, the old rule is closed with `EffectiveTo` set and its price untouched, an unknown id is rejected, and a rejected category leaves the existing rule active.

- [ ] **Step 15: Write `delivery/http/admin/pricing_rule_handler.go`**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	pricingadminuc "github.com/johnquangdev/laverte-home/usecase/pricingadmin"
)

type PricingRuleHandler struct {
	uc        pricingadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newPricingRuleHandler(uc pricingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *PricingRuleHandler {
	return &PricingRuleHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *PricingRuleHandler) create(c echo.Context) error {
	var req payload.UpsertPricingRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Create(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *PricingRuleHandler) supersede(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.UpsertPricingRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Supersede(c.Request().Context(), uint(id), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *PricingRuleHandler) list(c echo.Context) error {
	category := c.QueryParam("category")
	resp, err := h.uc.ListByCategory(c.Request().Context(), category)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
```

- [ ] **Step 16: Write `delivery/http/admin/pricing_rule_route.go`**

```go
package admin

import (
	"github.com/labstack/echo/v4"

	pricingadminuc "github.com/johnquangdev/laverte-home/usecase/pricingadmin"
)

func InitPricingRules(g *echo.Group, uc pricingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newPricingRuleHandler(uc, handleErr, handleOK)
	rules := g.Group("/pricing-rules")
	rules.GET("", h.list)
	rules.POST("", h.create)
	rules.PUT("/:id", h.supersede)
}
```

- [ ] **Step 17: Modify `delivery/http/http.go`** — add one field to `Deps` and one mount

Add the import `pricingadminuc "github.com/johnquangdev/laverte-home/usecase/pricingadmin"`, add this field to `Deps`:

```go
	PricingAdminUC pricingadminuc.IUseCase
```

and mount it on the shared `adminGroup`:

```go
	adminhttp.InitPricingRules(adminGroup, deps.PricingAdminUC, handleErr, handleOK)
```

- [ ] **Step 18: Modify `delivery/http/http_test.go`** — stub the new dependency

```go
type stubPricingAdminUC struct{}

func (stubPricingAdminUC) Create(context.Context, payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	return nil, nil
}
func (stubPricingAdminUC) Supersede(context.Context, uint, payload.UpsertPricingRuleRequest) (*presenter.PricingRuleResponse, error) {
	return nil, nil
}
func (stubPricingAdminUC) ListByCategory(context.Context, string) ([]presenter.PricingRuleResponse, error) {
	return nil, nil
}
```

and add `PricingAdminUC: stubPricingAdminUC{},` to the `Deps` literal in `newTestServer()`.

- [ ] **Step 19: Modify `cmd/main.go`** — wire the repo and both usecases

```go
	pricingRules := pricingrulerepo.NewPG(dbFactory)
	pricingAdminUC := pricingadminuc.New(pricingRules)
```

Add `PricingAdminUC: pricingAdminUC,` to the `httpserver.Deps{...}` literal, plus the `pricingrulerepo` and `pricingadminuc` imports. `pricingRules` is deliberately kept in scope here: Task 12 also builds `pricinguc.New(pricingRules)` from it for the booking usecase.

- [ ] **Step 20: Build, vet, test, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add model/pricing_rule.go migrations/0003_pricing_rules.sql repository/pricingrule usecase/pricing usecase/pricingadmin payload/pricing_rule.go presenter/pricing_rule.go delivery/http cmd/main.go
git commit -m "feat: PricingRule model + Compute pricing usecase + admin CRUD"
```

---

### Task 9: Booking model, migration with exclusion constraint, repository with conflict detection

**Files:**
- Create: `model/booking.go`
- Create: `migrations/0004_bookings.sql`
- Create: `repository/booking/interface.go`
- Create: `repository/booking/pg.go`
- Create: `repository/booking/pg_integration_test.go`

**Interfaces:**
- Produces: `model.Booking{...}` (full field set — see design doc §3; the lock-code columns `DoorLockCode`, `LockCodeAlertSentAt`, `LockCodeSentAt` land in the schema here but are first consumed in Tasks 16-17), `model.BookingType{Hourly,Overnight,Day}`, `model.BookingStatus{PendingPayment,Confirmed,Cancelled,Expired,Completed,NoShow}`.
- Produces: `bookingrepo.IRepository` with `Create`, `GetByID`, `Update`, `GetPendingByPhone`, `ListExpiredPending`, `ListByHomeAndDate`, `ListUpcomingMissingLockCode`, `ListReadyToSendLockCode`.
- Produces: `bookingrepo.ErrSlotConflict` — the sentinel `Create` returns when the DB exclusion constraint fires.

- [ ] **Step 1: Write `model/booking.go`**

```go
package model

import "time"

const (
	BookingTypeHourly    = "hourly"
	BookingTypeOvernight = "overnight"
	BookingTypeDay       = "day"

	BookingStatusPendingPayment = "pending_payment"
	BookingStatusConfirmed      = "confirmed"
	BookingStatusCancelled      = "cancelled"
	BookingStatusExpired        = "expired"
	BookingStatusCompleted      = "completed"
	BookingStatusNoShow         = "no_show"
)

type Booking struct {
	ID                    uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	HomeID                uint       `gorm:"not null;index" json:"home_id"`
	CustomerName          string     `gorm:"not null" json:"customer_name"`
	CustomerPhone         string     `gorm:"not null;index" json:"customer_phone"`
	StartTime             time.Time  `gorm:"not null" json:"start_time"`
	EndTime               time.Time  `gorm:"not null" json:"end_time"`
	BookingType           string     `gorm:"not null" json:"booking_type"`
	ComputedPrice         int64      `gorm:"not null" json:"computed_price"`
	Status                string     `gorm:"not null;index;default:'pending_payment'" json:"status"`
	PaymentID             *uint      `json:"payment_id"`
	GoogleCalendarEventID string     `json:"google_calendar_event_id"`
	DoorLockCode          *string    `json:"door_lock_code"`
	LockCodeAlertSentAt   *time.Time `json:"lock_code_alert_sent_at"`
	LockCodeSentAt        *time.Time `json:"lock_code_sent_at"`
	CreatedByAdminID      *uint      `json:"created_by_admin_id"`
	ExpiresAt             *time.Time `json:"expires_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (Booking) TableName() string { return "bookings" }

func IsValidBookingType(t string) bool {
	return t == BookingTypeHourly || t == BookingTypeOvernight || t == BookingTypeDay
}
```

- [ ] **Step 2: Write `migrations/0004_bookings.sql`** (`btree_gist` extension was already created in `0001_init.sql`)

```sql
-- +migrate Up
CREATE TABLE bookings (
    id BIGSERIAL PRIMARY KEY,
    home_id BIGINT NOT NULL REFERENCES homes(id),
    customer_name TEXT NOT NULL,
    customer_phone TEXT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    booking_type TEXT NOT NULL,
    computed_price BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending_payment',
    payment_id BIGINT,
    google_calendar_event_id TEXT NOT NULL DEFAULT '',
    door_lock_code TEXT,
    lock_code_alert_sent_at TIMESTAMPTZ,
    lock_code_sent_at TIMESTAMPTZ,
    created_by_admin_id BIGINT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    EXCLUDE USING gist (
        home_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    ) WHERE (status IN ('pending_payment', 'confirmed'))
);
CREATE INDEX idx_bookings_home_id ON bookings(home_id);
CREATE INDEX idx_bookings_customer_phone ON bookings(customer_phone);
CREATE INDEX idx_bookings_status ON bookings(status);

-- +migrate Down
DROP TABLE bookings;
```

- [ ] **Step 3: Write `repository/booking/interface.go`**

```go
package booking

import (
	"context"
	"errors"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

// ErrSlotConflict is returned by Create when the DB exclusion constraint
// rejects an overlapping [start_time, end_time) for the same home_id.
var ErrSlotConflict = errors.New("booking: slot conflict")

type IRepository interface {
	Create(ctx context.Context, b *model.Booking) error
	GetByID(ctx context.Context, id uint) (*model.Booking, error)
	Update(ctx context.Context, b *model.Booking) error
	GetPendingByPhone(ctx context.Context, phone string) (*model.Booking, error)
	ListExpiredPending(ctx context.Context, now time.Time) ([]*model.Booking, error)
	ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]*model.Booking, error)
	// ListUpcomingMissingLockCode returns confirmed bookings whose start_time
	// is within [now, now+leadTime) and that have no door_lock_code yet and
	// haven't been alerted on.
	ListUpcomingMissingLockCode(ctx context.Context, now time.Time, leadTime time.Duration) ([]*model.Booking, error)
	// ListReadyToSendLockCode returns confirmed bookings whose start_time has
	// already passed, that have a door_lock_code set, and haven't been sent yet.
	ListReadyToSendLockCode(ctx context.Context, now time.Time) ([]*model.Booking, error)
}
```

- [ ] **Step 4: Write `repository/booking/pg.go`**

```go
package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

// postgresExclusionViolation is the SQLSTATE Postgres raises when an EXCLUDE
// constraint rejects an insert/update — see migrations/0004_bookings.sql.
const postgresExclusionViolation = "23P01"

type pgRepository struct{ getDB func(context.Context) *gorm.DB }

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, b *model.Booking) error {
	err := r.getDB(ctx).Create(b).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresExclusionViolation {
		return ErrSlotConflict
	}
	return err
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.Booking, error) {
	var b model.Booking
	err := r.getDB(ctx).First(&b, id).Error
	return &b, err
}

func (r *pgRepository) Update(ctx context.Context, b *model.Booking) error {
	err := r.getDB(ctx).Save(b).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresExclusionViolation {
		return ErrSlotConflict
	}
	return err
}

func (r *pgRepository) GetPendingByPhone(ctx context.Context, phone string) (*model.Booking, error) {
	var b model.Booking
	err := r.getDB(ctx).
		Where("customer_phone = ? AND status = ? AND expires_at > ?", phone, model.BookingStatusPendingPayment, time.Now()).
		Order("created_at DESC").First(&b).Error
	return &b, err
}

func (r *pgRepository) ListExpiredPending(ctx context.Context, now time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).Where("status = ? AND expires_at < ?", model.BookingStatusPendingPayment, now).Find(&out).Error
	return out, err
}

func (r *pgRepository) ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]*model.Booking, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("home_id = ? AND start_time < ? AND end_time > ?", homeID, end, start).
		Order("start_time ASC").Find(&out).Error
	return out, err
}

func (r *pgRepository) ListUpcomingMissingLockCode(ctx context.Context, now time.Time, leadTime time.Duration) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("status = ? AND door_lock_code IS NULL AND lock_code_alert_sent_at IS NULL AND start_time <= ? AND start_time > ?",
			model.BookingStatusConfirmed, now.Add(leadTime), now).
		Find(&out).Error
	return out, err
}

func (r *pgRepository) ListReadyToSendLockCode(ctx context.Context, now time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	err := r.getDB(ctx).
		Where("status = ? AND door_lock_code IS NOT NULL AND lock_code_sent_at IS NULL AND start_time <= ?",
			model.BookingStatusConfirmed, now).
		Find(&out).Error
	return out, err
}
```

- [ ] **Step 5: Write `repository/booking/pg_integration_test.go`** (this is the test that proves the exclusion constraint works — requires the docker-compose Postgres from Task 2)

```go
package booking

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/migrations"
	"github.com/johnquangdev/laverte-home/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	if _, err := migrate.Exec(sqlDB, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() {
		// Truncate rather than migrate-down: these tests share one database, so
		// each needs a clean slate without tearing the schema out from under a
		// sibling test.
		if _, err := sqlDB.Exec("TRUNCATE bookings, homes RESTART IDENTITY CASCADE"); err != nil {
			t.Errorf("truncate: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gormDB
}

func TestCreateRejectsOverlappingBookingForSameHome(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	end := start.Add(2 * time.Hour)

	first := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: end, BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	overlapping := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start.Add(30 * time.Minute), EndTime: end.Add(30 * time.Minute),
		BookingType: model.BookingTypeHourly, ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
	}
	if err := repo.Create(ctx, overlapping); err != ErrSlotConflict {
		t.Fatalf("second Create() error = %v, want ErrSlotConflict", err)
	}
}

func TestCreateAllowsNonOverlappingBookingForSameHome(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 2", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	first := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	after := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start.Add(time.Hour), EndTime: start.Add(2 * time.Hour),
		BookingType: model.BookingTypeHourly, ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
	}
	if err := repo.Create(ctx, after); err != nil {
		t.Fatalf("adjacent (non-overlapping) Create() error = %v, want nil", err)
	}
}

func TestCreateAllowsOverlapWhenFirstIsCancelled(t *testing.T) {
	db := setupTestDB(t)
	getDB := func(context.Context) *gorm.DB { return db }
	repo := NewPG(getDB)
	ctx := context.Background()

	home := &model.Home{Name: "Test Home 3", Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	cancelled := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusCancelled,
	}
	if err := repo.Create(ctx, cancelled); err != nil {
		t.Fatalf("create cancelled booking: %v", err)
	}

	overlapping := &model.Booking{
		HomeID: home.ID, CustomerName: "B", CustomerPhone: "0900000002",
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: 100000, Status: model.BookingStatusPendingPayment,
	}
	if err := repo.Create(ctx, overlapping); err != nil {
		t.Fatalf("Create() overlapping a cancelled booking error = %v, want nil", err)
	}
}
```

- [ ] **Step 6: Run the integration test against docker-compose Postgres**

Run: `TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test ./repository/booking/... -v`
Expected: PASS on all three tests — confirms the exclusion constraint blocks true overlaps, allows adjacent/non-overlapping bookings, and ignores cancelled rows.

- [ ] **Step 7: Build, vet, commit**

```bash
go build ./...
go vet ./...
git add model/booking.go migrations/0004_bookings.sql repository/booking
git commit -m "feat: Booking model + exclusion-constraint-backed repository"
```

---

## Phase 2 — Blocked slots, payment rail, and the guest booking flow

### Task 10: BlockedSlot model, migration, repository, admin CRUD

**Files:**
- Create: `model/blocked_slot.go`
- Create: `migrations/0005_blocked_slots.sql`
- Create: `repository/blockedslot/interface.go`
- Create: `repository/blockedslot/pg.go`
- Create: `payload/blocked_slot.go`
- Create: `presenter/blocked_slot.go`
- Create: `usecase/blockedslot/interface.go`
- Create: `usecase/blockedslot/usecase.go`
- Create: `usecase/blockedslot/usecase_test.go`
- Create: `delivery/http/admin/blocked_slot_handler.go`
- Create: `delivery/http/admin/blocked_slot_route.go`
- Modify: `delivery/http/http.go` — add a `BlockedSlotUC` field to `Deps`, mount `InitBlockedSlots`
- Modify: `delivery/http/http_test.go` — add the stub usecase the new param needs
- Modify: `cmd/main.go` — wire `repository/blockedslot` + `usecase/blockedslot`

**Interfaces:**
- Consumes: `middleware.ClaimsFromContext` (Task 5); `HandleErrFunc`/`HandleOKFunc` already declared in package `delivery/http/admin` (Task 6) — do NOT redeclare them.
- Produces: `model.BlockedSlot{ID, HomeID, StartTime, EndTime, Reason, CreatedByAdminID, CreatedAt}`.
- Produces: `blockedslotrepo.IRepository` (`Create`, `Delete`, `ListByHome`, `HasOverlap`) — `HasOverlap` is the method Task 12's booking usecase calls before inserting a guest booking.
- Produces: `blockedslotuc.IUseCase` (`Create`, `Delete`, `ListByHome`).

- [ ] **Step 1: Write `model/blocked_slot.go`**

```go
package model

import "time"

type BlockedSlot struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	HomeID           uint      `gorm:"not null;index" json:"home_id"`
	StartTime        time.Time `gorm:"not null" json:"start_time"`
	EndTime          time.Time `gorm:"not null" json:"end_time"`
	Reason           string    `json:"reason"`
	CreatedByAdminID *uint     `json:"created_by_admin_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func (BlockedSlot) TableName() string { return "blocked_slots" }
```

- [ ] **Step 2: Write `migrations/0005_blocked_slots.sql`**

```sql
-- +migrate Up
CREATE TABLE blocked_slots (
    id BIGSERIAL PRIMARY KEY,
    home_id BIGINT NOT NULL REFERENCES homes(id),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by_admin_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Deliberately no EXCLUDE constraint, unlike bookings: admins legitimately
-- stack overlapping blocks (a maintenance window nested inside a longer
-- seasonal close), and readers only ever ask whether ANY block overlaps.
CREATE INDEX idx_blocked_slots_home_id ON blocked_slots(home_id);

-- +migrate Down
DROP TABLE blocked_slots;
```

- [ ] **Step 3: Write `repository/blockedslot/interface.go`**

```go
package blockedslot

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, s *model.BlockedSlot) error
	Delete(ctx context.Context, id uint) error
	ListByHome(ctx context.Context, homeID uint) ([]*model.BlockedSlot, error)
	// HasOverlap treats both ranges as half-open [start, end), so a block that
	// ends exactly when a booking starts is not a conflict.
	HasOverlap(ctx context.Context, homeID uint, start, end time.Time) (bool, error)
}
```

- [ ] **Step 4: Write `repository/blockedslot/pg.go`**

```go
package blockedslot

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct{ getDB func(context.Context) *gorm.DB }

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, s *model.BlockedSlot) error {
	return r.getDB(ctx).Create(s).Error
}

func (r *pgRepository) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&model.BlockedSlot{}, id).Error
}

func (r *pgRepository) ListByHome(ctx context.Context, homeID uint) ([]*model.BlockedSlot, error) {
	var out []*model.BlockedSlot
	err := r.getDB(ctx).Where("home_id = ?", homeID).Order("start_time ASC").Find(&out).Error
	return out, err
}

func (r *pgRepository) HasOverlap(ctx context.Context, homeID uint, start, end time.Time) (bool, error) {
	var count int64
	err := r.getDB(ctx).Model(&model.BlockedSlot{}).
		Where("home_id = ? AND start_time < ? AND end_time > ?", homeID, end, start).
		Count(&count).Error
	return count > 0, err
}
```

- [ ] **Step 5: Write `payload/blocked_slot.go`**

```go
package payload

import "time"

type CreateBlockedSlotRequest struct {
	HomeID    uint      `json:"home_id" validate:"required"`
	StartTime time.Time `json:"start_time" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required"`
	Reason    string    `json:"reason"`
}
```

- [ ] **Step 6: Write `presenter/blocked_slot.go`**

```go
package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type BlockedSlotResponse struct {
	ID               uint      `json:"id"`
	HomeID           uint      `json:"home_id"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Reason           string    `json:"reason"`
	CreatedByAdminID *uint     `json:"created_by_admin_id"`
}

func ToBlockedSlotResponse(s *model.BlockedSlot) BlockedSlotResponse {
	return BlockedSlotResponse{
		ID: s.ID, HomeID: s.HomeID, StartTime: s.StartTime, EndTime: s.EndTime,
		Reason: s.Reason, CreatedByAdminID: s.CreatedByAdminID,
	}
}
```

- [ ] **Step 7: Write `usecase/blockedslot/interface.go`**

```go
package blockedslot

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Create(ctx context.Context, req payload.CreateBlockedSlotRequest, adminID uint) (*presenter.BlockedSlotResponse, error)
	Delete(ctx context.Context, id uint) error
	ListByHome(ctx context.Context, homeID uint) ([]presenter.BlockedSlotResponse, error)
}
```

- [ ] **Step 8: Write `usecase/blockedslot/usecase.go`**

```go
package blockedslot

import (
	"context"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
)

type UseCase struct{ repo blockedslotrepo.IRepository }

func New(repo blockedslotrepo.IRepository) IUseCase { return &UseCase{repo: repo} }

func (uc *UseCase) Create(ctx context.Context, req payload.CreateBlockedSlotRequest, adminID uint) (*presenter.BlockedSlotResponse, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.Validation("end_time phai sau start_time")
	}
	s := &model.BlockedSlot{
		HomeID: req.HomeID, StartTime: req.StartTime, EndTime: req.EndTime,
		Reason: req.Reason, CreatedByAdminID: &adminID,
	}
	if err := uc.repo.Create(ctx, s); err != nil {
		return nil, apperr.Internal(err)
	}
	resp := presenter.ToBlockedSlotResponse(s)
	return &resp, nil
}

func (uc *UseCase) Delete(ctx context.Context, id uint) error {
	if err := uc.repo.Delete(ctx, id); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

func (uc *UseCase) ListByHome(ctx context.Context, homeID uint) ([]presenter.BlockedSlotResponse, error) {
	slots, err := uc.repo.ListByHome(ctx, homeID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.BlockedSlotResponse, 0, len(slots))
	for _, s := range slots {
		out = append(out, presenter.ToBlockedSlotResponse(s))
	}
	return out, nil
}
```

- [ ] **Step 9: Write `usecase/blockedslot/usecase_test.go`**

```go
package blockedslot

import (
	"context"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
)

type fakeRepo struct {
	slots  map[uint]*model.BlockedSlot
	nextID uint
}

func newFakeRepo() *fakeRepo { return &fakeRepo{slots: map[uint]*model.BlockedSlot{}} }

func (f *fakeRepo) Create(_ context.Context, s *model.BlockedSlot) error {
	f.nextID++
	s.ID = f.nextID
	f.slots[s.ID] = s
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, id uint) error {
	delete(f.slots, id)
	return nil
}

func (f *fakeRepo) ListByHome(_ context.Context, homeID uint) ([]*model.BlockedSlot, error) {
	out := make([]*model.BlockedSlot, 0, len(f.slots))
	for _, s := range f.slots {
		if s.HomeID == homeID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) HasOverlap(_ context.Context, homeID uint, start, end time.Time) (bool, error) {
	for _, s := range f.slots {
		if s.HomeID == homeID && s.StartTime.Before(end) && s.EndTime.After(start) {
			return true, nil
		}
	}
	return false, nil
}

func TestCreateRejectsEndBeforeStart(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := payload.CreateBlockedSlotRequest{HomeID: 1, StartTime: start, EndTime: start.Add(-time.Hour)}

	if _, err := uc.Create(context.Background(), req, 7); err == nil {
		t.Fatal("Create() with end before start = nil error, want error")
	}
}

func TestCreateRejectsZeroLengthSlot(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := payload.CreateBlockedSlotRequest{HomeID: 1, StartTime: start, EndTime: start}

	if _, err := uc.Create(context.Background(), req, 7); err == nil {
		t.Fatal("Create() with end == start = nil error, want error")
	}
}

func TestCreateThenListByHomeRoundTrip(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := payload.CreateBlockedSlotRequest{HomeID: 3, StartTime: start, EndTime: start.Add(4 * time.Hour), Reason: "bao tri dieu hoa"}

	created, err := uc.Create(context.Background(), req, 7)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.CreatedByAdminID == nil || *created.CreatedByAdminID != 7 {
		t.Errorf("CreatedByAdminID = %v, want 7", created.CreatedByAdminID)
	}

	list, err := uc.ListByHome(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListByHome() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID || list[0].Reason != "bao tri dieu hoa" {
		t.Errorf("ListByHome() = %+v, want one item with ID %d", list, created.ID)
	}

	other, err := uc.ListByHome(context.Background(), 99)
	if err != nil {
		t.Fatalf("ListByHome(99) error = %v", err)
	}
	if len(other) != 0 {
		t.Errorf("ListByHome(99) = %+v, want empty", other)
	}
}

func TestDeleteRemovesSlot(t *testing.T) {
	uc := New(newFakeRepo())
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	created, err := uc.Create(context.Background(), payload.CreateBlockedSlotRequest{
		HomeID: 3, StartTime: start, EndTime: start.Add(time.Hour),
	}, 7)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := uc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	list, err := uc.ListByHome(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListByHome() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListByHome() after Delete = %+v, want empty", list)
	}
}
```

- [ ] **Step 10: Run tests to verify they pass**

Run: `go test ./usecase/blockedslot/... -v`
Expected: PASS (4 tests)

- [ ] **Step 11: Write `delivery/http/admin/blocked_slot_handler.go`**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
)

type BlockedSlotHandler struct {
	uc        blockedslotuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newBlockedSlotHandler(uc blockedslotuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *BlockedSlotHandler {
	return &BlockedSlotHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *BlockedSlotHandler) list(c echo.Context) error {
	homeID, err := strconv.ParseUint(c.QueryParam("home_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid home_id"})
	}
	resp, err := h.uc.ListByHome(c.Request().Context(), uint(homeID))
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BlockedSlotHandler) create(c echo.Context) error {
	var req payload.CreateBlockedSlotRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	adminID := middleware.ClaimsFromContext(c).UserID
	resp, err := h.uc.Create(c.Request().Context(), req, adminID)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BlockedSlotHandler) remove(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	if err := h.uc.Delete(c.Request().Context(), uint(id)); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
```

- [ ] **Step 12: Write `delivery/http/admin/blocked_slot_route.go`**

```go
package admin

import (
	"github.com/labstack/echo/v4"

	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
)

func InitBlockedSlots(g *echo.Group, uc blockedslotuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newBlockedSlotHandler(uc, handleErr, handleOK)
	slots := g.Group("/blocked-slots")
	slots.GET("", h.list)
	slots.POST("", h.create)
	slots.DELETE("/:id", h.remove)
}
```

- [ ] **Step 13: Modify `delivery/http/http.go`** — add the usecase parameter and mount the routes

Add to the import block:

```go
	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
```

Add one field to the `Deps` struct (Task 6):

```go
	BlockedSlotUC blockedslotuc.IUseCase
```

Add this call immediately after the existing `adminhttp.InitPricingRules(...)` line:

```go
	adminhttp.InitBlockedSlots(adminGroup, deps.BlockedSlotUC, handleErr, handleOK)
```

- [ ] **Step 14: Modify `delivery/http/http_test.go`** — add the stub the new parameter needs

Append this stub next to the existing ones:

```go
type stubBlockedSlotUC struct{}

func (stubBlockedSlotUC) Create(context.Context, payload.CreateBlockedSlotRequest, uint) (*presenter.BlockedSlotResponse, error) {
	return nil, nil
}
func (stubBlockedSlotUC) Delete(context.Context, uint) error { return nil }
func (stubBlockedSlotUC) ListByHome(context.Context, uint) ([]presenter.BlockedSlotResponse, error) {
	return nil, nil
}
```

and add `BlockedSlotUC: stubBlockedSlotUC{},` to the `Deps` literal inside `newTestServer()`.

- [ ] **Step 15: Modify `cmd/main.go`** — wire the repository and usecase

Add to the import block:

```go
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
	blockedslotuc "github.com/johnquangdev/laverte-home/usecase/blockedslot"
```

Add next to the other repo/usecase constructions:

```go
	blockedSlots := blockedslotrepo.NewPG(dbFactory)
	blockedSlotUC := blockedslotuc.New(blockedSlots)
```

and add `BlockedSlotUC: blockedSlotUC,` to the `httpserver.Deps{...}` literal.

- [ ] **Step 16: Verify migration 0005 applies and rolls back against docker-compose Postgres**

Run: `TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test ./migrations/... -v`
Expected: PASS — `TestMigrationsApplyCleanly` now walks 0001→0005 up then all the way down, proving `blocked_slots`' FK to `homes(id)` drops in the right order.

- [ ] **Step 17: Build, vet, test, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add model/blocked_slot.go migrations/0005_blocked_slots.sql repository/blockedslot payload/blocked_slot.go presenter/blocked_slot.go usecase/blockedslot delivery/http cmd/main.go
git commit -m "feat: BlockedSlot model + repository with HasOverlap + admin CRUD"
```

---

### Task 11: Payment model, migration, repository, SePay VietQR client

**Files:**
- Create: `model/payment.go`
- Create: `migrations/0006_payments.sql`
- Create: `repository/payment/interface.go`
- Create: `repository/payment/pg.go`
- Create: `repository/payment/pg_integration_test.go`
- Create: `util/checkout/interface.go`
- Create: `util/checkout/sepay.go`
- Create: `util/checkout/sepay_test.go`

**Interfaces:**
- Consumes: `config.Config` fields `SePayBankAccount`, `SePayBankCode`, `SePayWebhookSecret`, `SePayTransferPrefix` (Task 1); `migrations.FS` (Task 2); `model.Home`/`model.Booking` for the integration test's FK rows (Tasks 7 and 9).
- Produces: `model.Payment{ID, BookingID, Provider, Amount, Status, QRContent, SePayTransactionRef, PaidAt, CreatedAt}`, consts `model.PaymentProviderSePay/PaymentProviderCash`, `model.PaymentStatusPending/Paid/Expired/Failed`.
- Produces: `paymentrepo.IRepository` (`Create`, `GetByID`, `GetByBookingID`, `MarkPaidIfPending`, `SumPaidBetween`) — `MarkPaidIfPending` is the once-only settlement guard Task 13's webhook usecase calls; `SumPaidBetween` feeds the admin revenue overview.
- Produces: `checkout.IPaymentProvider` (`CreateQR`, `VerifyWebhook`), `checkout.CreateQRRequest`, `checkout.QRResult`, `checkout.WebhookEvent`, `checkout.ErrWebhookPing`, `checkout.NewSePay(cfg config.Config) IPaymentProvider`.

- [ ] **Step 1: Write `model/payment.go`**

```go
package model

import "time"

const (
	PaymentProviderSePay = "sepay"
	PaymentProviderCash  = "cash"

	PaymentStatusPending = "pending"
	PaymentStatusPaid    = "paid"
	PaymentStatusExpired = "expired"
	PaymentStatusFailed  = "failed"
)

type Payment struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	BookingID uint   `gorm:"not null;index" json:"booking_id"`
	Provider  string `gorm:"not null" json:"provider"`
	Amount    int64  `gorm:"not null" json:"amount"`
	Status    string `gorm:"not null;index;default:'pending'" json:"status"`
	QRContent string `gorm:"column:qr_content" json:"qr_content"`
	// SePayTransactionRef holds SePay's own transaction id, stable across
	// webhook redeliveries — it is the dedup key, not the transfer memo.
	SePayTransactionRef string     `gorm:"column:sepay_transaction_ref" json:"sepay_transaction_ref"`
	PaidAt              *time.Time `json:"paid_at"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (Payment) TableName() string { return "payments" }
```

- [ ] **Step 2: Write `migrations/0006_payments.sql`**

```sql
-- +migrate Up
CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL REFERENCES bookings(id),
    provider TEXT NOT NULL,
    amount BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    qr_content TEXT NOT NULL DEFAULT '',
    sepay_transaction_ref TEXT NOT NULL DEFAULT '',
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_booking_id ON payments(booking_id);
CREATE INDEX idx_payments_status ON payments(status);
-- A redelivered webhook carries the same SePay transaction id, so this makes a
-- second settlement of the same bank transfer fail at the DB even if it lands
-- on a different payment row. Rows no webhook has touched keep '' and are
-- excluded, since many of them coexist legitimately.
CREATE UNIQUE INDEX uq_payments_sepay_transaction_ref
    ON payments(sepay_transaction_ref) WHERE sepay_transaction_ref <> '';

-- +migrate Down
DROP TABLE payments;
```

- [ ] **Step 3: Verify migration 0006 applies and rolls back**

Run: `TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test ./migrations/... -v`
Expected: PASS — proves the partial unique index and the `bookings(id)` FK apply and drop cleanly in sequence with 0001→0005.

- [ ] **Step 4: Write `repository/payment/interface.go`**

```go
package payment

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type IRepository interface {
	Create(ctx context.Context, p *model.Payment) error
	GetByID(ctx context.Context, id uint) (*model.Payment, error)
	GetByBookingID(ctx context.Context, bookingID uint) (*model.Payment, error)
	// MarkPaidIfPending settles a payment exactly once: it reports false (with
	// a nil error) when the row was no longer pending, which is the normal
	// outcome for a redelivered webhook and must not be treated as a failure.
	MarkPaidIfPending(ctx context.Context, paymentID uint, externalRef string, paidAt time.Time) (bool, error)
	// SumPaidBetween totals paid amounts over [from, to).
	SumPaidBetween(ctx context.Context, from, to time.Time) (int64, error)
}
```

- [ ] **Step 5: Write `repository/payment/pg.go`**

```go
package payment

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/model"
)

type pgRepository struct{ getDB func(context.Context) *gorm.DB }

func NewPG(getDB func(context.Context) *gorm.DB) IRepository { return &pgRepository{getDB} }

func (r *pgRepository) Create(ctx context.Context, p *model.Payment) error {
	return r.getDB(ctx).Create(p).Error
}

func (r *pgRepository) GetByID(ctx context.Context, id uint) (*model.Payment, error) {
	var p model.Payment
	err := r.getDB(ctx).First(&p, id).Error
	return &p, err
}

func (r *pgRepository) GetByBookingID(ctx context.Context, bookingID uint) (*model.Payment, error) {
	var p model.Payment
	err := r.getDB(ctx).Where("booking_id = ?", bookingID).Order("id DESC").First(&p).Error
	return &p, err
}

// MarkPaidIfPending puts the pending-check inside the UPDATE's WHERE clause so
// two concurrent webhook deliveries cannot both observe "pending" and settle;
// Postgres serializes them on the row lock and the loser matches zero rows.
func (r *pgRepository) MarkPaidIfPending(ctx context.Context, paymentID uint, externalRef string, paidAt time.Time) (bool, error) {
	result := r.getDB(ctx).Model(&model.Payment{}).
		Where("id = ? AND status = ?", paymentID, model.PaymentStatusPending).
		Updates(map[string]any{
			"status":                model.PaymentStatusPaid,
			"sepay_transaction_ref": externalRef,
			"paid_at":               paidAt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *pgRepository) SumPaidBetween(ctx context.Context, from, to time.Time) (int64, error) {
	var total int64
	err := r.getDB(ctx).Model(&model.Payment{}).
		Where("status = ? AND paid_at >= ? AND paid_at < ?", model.PaymentStatusPaid, from, to).
		Select("COALESCE(SUM(amount), 0)").Scan(&total).Error
	return total, err
}
```

- [ ] **Step 6: Write `repository/payment/pg_integration_test.go`**

```go
package payment

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/migrations"
	"github.com/johnquangdev/laverte-home/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	if _, err := migrate.Exec(sqlDB, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() {
		// Truncate rather than migrate-down: these tests share one database, so
		// each needs a clean slate without tearing the schema out from under a
		// sibling test.
		if _, err := sqlDB.Exec("TRUNCATE payments, bookings, homes RESTART IDENTITY CASCADE"); err != nil {
			t.Errorf("truncate: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gormDB
}

// seedPendingPayment inserts the home+booking rows the payments FK requires,
// then a pending sepay payment for that booking.
func seedPendingPayment(t *testing.T, db *gorm.DB, repo IRepository, phone string, amount int64) *model.Payment {
	t.Helper()
	ctx := context.Background()

	home := &model.Home{Name: "Payment Test Home " + phone, Category: model.HomeCategoryHome, IsActive: true}
	if err := db.Create(home).Error; err != nil {
		t.Fatalf("create home: %v", err)
	}

	start := time.Now().Add(time.Hour).Truncate(time.Second)
	booking := &model.Booking{
		HomeID: home.ID, CustomerName: "A", CustomerPhone: phone,
		StartTime: start, EndTime: start.Add(2 * time.Hour), BookingType: model.BookingTypeHourly,
		ComputedPrice: amount, Status: model.BookingStatusPendingPayment,
	}
	if err := db.Create(booking).Error; err != nil {
		t.Fatalf("create booking: %v", err)
	}

	p := &model.Payment{
		BookingID: booking.ID, Provider: model.PaymentProviderSePay, Amount: amount,
		Status: model.PaymentStatusPending, QRContent: "https://vietqr.app/img?acc=1&bank=MSB",
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create payment: %v", err)
	}
	return p
}

func TestMarkPaidIfPendingSettlesOnceThenReportsFalse(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPG(func(context.Context) *gorm.DB { return db })
	ctx := context.Background()

	p := seedPendingPayment(t, db, repo, "0900000001", 350000)
	paidAt := time.Now().Truncate(time.Second)

	settled, err := repo.MarkPaidIfPending(ctx, p.ID, "998877", paidAt)
	if err != nil {
		t.Fatalf("first MarkPaidIfPending() error = %v", err)
	}
	if !settled {
		t.Fatal("first MarkPaidIfPending() = false, want true")
	}

	// A redelivered webhook must be a no-op, not an error.
	settled, err = repo.MarkPaidIfPending(ctx, p.ID, "998877", paidAt)
	if err != nil {
		t.Fatalf("second MarkPaidIfPending() error = %v", err)
	}
	if settled {
		t.Fatal("second MarkPaidIfPending() = true, want false")
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Status != model.PaymentStatusPaid {
		t.Errorf("Status = %q, want %q", got.Status, model.PaymentStatusPaid)
	}
	if got.SePayTransactionRef != "998877" {
		t.Errorf("SePayTransactionRef = %q, want 998877", got.SePayTransactionRef)
	}
	if got.PaidAt == nil {
		t.Error("PaidAt = nil, want a timestamp")
	}
}

func TestGetByBookingIDAndSumPaidBetween(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPG(func(context.Context) *gorm.DB { return db })
	ctx := context.Background()

	paid := seedPendingPayment(t, db, repo, "0900000002", 500000)
	pending := seedPendingPayment(t, db, repo, "0900000003", 250000)

	paidAt := time.Now().Truncate(time.Second)
	if _, err := repo.MarkPaidIfPending(ctx, paid.ID, "112233", paidAt); err != nil {
		t.Fatalf("MarkPaidIfPending() error = %v", err)
	}

	got, err := repo.GetByBookingID(ctx, paid.BookingID)
	if err != nil {
		t.Fatalf("GetByBookingID() error = %v", err)
	}
	if got.ID != paid.ID {
		t.Errorf("GetByBookingID().ID = %d, want %d", got.ID, paid.ID)
	}

	total, err := repo.SumPaidBetween(ctx, paidAt.Add(-time.Hour), paidAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("SumPaidBetween() error = %v", err)
	}
	if total != 500000 {
		t.Errorf("SumPaidBetween() = %d, want 500000 (the still-pending %d must not count)", total, pending.Amount)
	}

	empty, err := repo.SumPaidBetween(ctx, paidAt.Add(24*time.Hour), paidAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("SumPaidBetween() over an empty window error = %v", err)
	}
	if empty != 0 {
		t.Errorf("SumPaidBetween() over an empty window = %d, want 0", empty)
	}
}
```

- [ ] **Step 7: Run the payment repository integration tests**

Run: `TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test ./repository/payment/... -v`
Expected: PASS on both tests — confirms the conditional UPDATE settles exactly once and that `SumPaidBetween` counts only paid rows inside the window.

- [ ] **Step 8: Write `util/checkout/interface.go`**

```go
package checkout

import (
	"context"
	"errors"
	"net/http"
)

type CreateQRRequest struct {
	BookingID uint
	AmountVND int64
}

type QRResult struct {
	QRContent   string // URL or payload the FE renders as a QR image
	ProviderRef string // memo we expect back in the bank transfer content
}

type WebhookEvent struct {
	ProviderRef string // memo parsed out of the transfer content
	ExternalRef string // SePay's own stable transaction id (dedup key)
	Success     bool
	AmountVND   int64
	Raw         string
}

// ErrWebhookPing marks a provider connectivity check rather than a payment —
// the caller must acknowledge it with 200 and settle nothing.
var ErrWebhookPing = errors.New("checkout: webhook connectivity ping")

type IPaymentProvider interface {
	CreateQR(ctx context.Context, req CreateQRRequest) (*QRResult, error)
	VerifyWebhook(ctx context.Context, raw []byte, headers http.Header) (*WebhookEvent, error)
}
```

- [ ] **Step 9: Write `util/checkout/sepay.go`**

```go
package checkout

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/johnquangdev/laverte-home/config"
)

type sepayProvider struct {
	bankAccount    string
	bankCode       string
	webhookSecret  string
	transferPrefix string
}

func NewSePay(cfg config.Config) IPaymentProvider {
	return &sepayProvider{
		bankAccount:    cfg.SePayBankAccount,
		bankCode:       cfg.SePayBankCode,
		webhookSecret:  cfg.SePayWebhookSecret,
		transferPrefix: cfg.SePayTransferPrefix,
	}
}

// alphanumUpper keeps only what reliably survives a bank transfer's content
// field: banking apps strip or reject punctuation, and some echo the memo back
// uppercased, so a same-memo lookup must not depend on either.
func alphanumUpper(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bookingMemo derives the transfer memo for a booking (e.g. "LAVERTE42").
// Deriving it from the booking ID instead of a random nonce means a webhook can
// be matched back to its booking without a lookup table.
func bookingMemo(prefix string, bookingID uint) string {
	return alphanumUpper(prefix) + strconv.FormatUint(uint64(bookingID), 10)
}

// parseBookingMemo recovers a bookingMemo from a bank's transfer content, which
// arrives wrapped in free text and with punctuation gone ("CT DEN:LAVERTE42
// chuyen tien"). The digit run stops at the first non-digit so trailing words
// can't be absorbed into the booking id. Returns "" when nothing matches.
func parseBookingMemo(prefix, content string) string {
	cleanPrefix := alphanumUpper(prefix)
	if cleanPrefix == "" {
		return ""
	}
	cleaned := alphanumUpper(content)
	i := strings.Index(cleaned, cleanPrefix)
	if i < 0 {
		return ""
	}
	rest := cleaned[i+len(cleanPrefix):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return ""
	}
	return cleanPrefix + rest[:end]
}

// CreateQR returns a URL-based QR renderer rather than a hand-rolled EMVCo
// payload. vietqr.app/img is the endpoint SePay's own docs document for this
// (developer.sepay.vn/vi/tien-ich-khac/tao-qr-code).
func (p *sepayProvider) CreateQR(_ context.Context, req CreateQRRequest) (*QRResult, error) {
	if p.bankAccount == "" || p.bankCode == "" {
		return nil, errors.New("sepay: bank account/code not configured")
	}
	// Without a prefix the memo would be bare digits, which any unrelated
	// transfer whose content happens to contain that number would match.
	if alphanumUpper(p.transferPrefix) == "" {
		return nil, errors.New("sepay: transfer prefix not configured")
	}

	memo := bookingMemo(p.transferPrefix, req.BookingID)
	qrURL := fmt.Sprintf("https://vietqr.app/img?acc=%s&bank=%s&amount=%d&des=%s",
		url.QueryEscape(p.bankAccount), url.QueryEscape(p.bankCode), req.AmountVND, url.QueryEscape(memo))
	return &QRResult{QRContent: qrURL, ProviderRef: memo}, nil
}

// sepayWebhookPayload is the subset of SePay's webhook body settlement needs.
// transferType is "in" for a credit (money received) and "out" for a debit.
// ID is documented as the value that does not change across retries and
// replays, i.e. stable per redelivery but unique per actual transfer — unlike
// Content, which a second real transfer can legitimately repeat.
type sepayWebhookPayload struct {
	ID             int64  `json:"id"`
	AccountNumber  string `json:"accountNumber"`
	Code           string `json:"code"`
	Content        string `json:"content"`
	TransferAmount int64  `json:"transferAmount"`
	TransferType   string `json:"transferType"`
}

// sepayTestWebhookContentMarker is the fixed Vietnamese phrase SePay's
// dashboard "Test" button always sends as its connectivity-check content.
// accountNumber can't be used to detect the ping — it reuses a plausible real
// account — and without this check the ping passes HMAC verification, then
// falls through to a memo lookup that never matches and surfaces as a
// misleading "malformed payload" instead of the harmless ping it is.
const sepayTestWebhookContentMarker = "giao dich thu nghiem"

func isSePayWebhookPing(payload sepayWebhookPayload) bool {
	return strings.Contains(strings.ToLower(payload.Content), sepayTestWebhookContentMarker)
}

// sepayReplayWindow bounds how stale a signed webhook's timestamp may be —
// SePay's docs (developer.sepay.vn/vi/sepay-webhooks/xac-thuc) treat requests
// more than 5 minutes off the current time as a replay risk.
const sepayReplayWindow = 5 * time.Minute

// verifySePayHMAC implements SePay's recommended webhook auth scheme:
// HMAC-SHA256 over "<unix-timestamp>.<raw body>", sent as
// "X-SePay-Signature: sha256=<hex>" alongside "X-SePay-Timestamp". Must run
// against the raw, unparsed body — re-marshaling would silently break the
// signature on any key-order or whitespace difference.
func verifySePayHMAC(raw []byte, timestampHeader, signatureHeader, secret string) error {
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return errors.New("sepay: missing or malformed X-SePay-Timestamp")
	}
	if age := time.Since(time.Unix(ts, 0)); age > sepayReplayWindow || age < -sepayReplayWindow {
		return errors.New("sepay: webhook timestamp outside replay window")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestampHeader))
	mac.Write([]byte("."))
	mac.Write(raw)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(signatureHeader)) != 1 {
		return errors.New("sepay: invalid webhook signature")
	}
	return nil
}

// VerifyWebhook accepts only HMAC-SHA256-authenticated deliveries: SePay also
// offers API-key and OAuth2 modes, but only HMAC binds the raw payload to a
// timestamp and so protects integrity and stale replay at once. Configure the
// SePay dashboard endpoint accordingly.
//
// Code is preferred over Content for the memo because SePay documents content
// as the unprocessed bank memo while code is the field it extracted per the
// merchant's payment-code configuration.
func (p *sepayProvider) VerifyWebhook(_ context.Context, raw []byte, headers http.Header) (*WebhookEvent, error) {
	if p.webhookSecret == "" {
		return nil, errors.New("sepay: webhook secret not configured")
	}

	if err := verifySePayHMAC(raw, headers.Get("X-SePay-Timestamp"), headers.Get("X-SePay-Signature"), p.webhookSecret); err != nil {
		return nil, err
	}

	var payload sepayWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("sepay: malformed webhook payload")
	}

	if isSePayWebhookPing(payload) {
		return nil, ErrWebhookPing
	}

	memo := parseBookingMemo(p.transferPrefix, payload.Code)
	if memo == "" {
		memo = parseBookingMemo(p.transferPrefix, payload.Content)
	}
	if memo == "" {
		return nil, errors.New("sepay: no recognizable booking memo in transfer content")
	}

	return &WebhookEvent{
		ProviderRef: memo,
		ExternalRef: strconv.FormatInt(payload.ID, 10),
		Success:     payload.TransferType == "in",
		AmountVND:   payload.TransferAmount,
		Raw:         string(raw),
	}, nil
}
```

- [ ] **Step 10: Write `util/checkout/sepay_test.go`**

```go
package checkout

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/config"
)

const testWebhookSecret = "top-secret"

func testSePay() IPaymentProvider {
	return NewSePay(config.Config{
		SePayBankAccount:    "0123456789",
		SePayBankCode:       "MSB",
		SePayWebhookSecret:  testWebhookSecret,
		SePayTransferPrefix: "LAVERTE",
	})
}

// signedHeaders recomputes the HMAC the same way SePay does, so the test stays
// self-consistent instead of asserting a hard-coded hex digest.
func signedHeaders(body []byte, secret string, at time.Time) http.Header {
	tsStr := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(body)

	h := http.Header{}
	h.Set("X-SePay-Timestamp", tsStr)
	h.Set("X-SePay-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return h
}

func paidWebhookBody() []byte {
	return []byte(`{"id":998877,"accountNumber":"0123456789","code":"LAVERTE42","content":"CT DEN:LAVERTE42 chuyen tien","transferAmount":350000,"transferType":"in"}`)
}

func TestCreateQRBuildsVietQRURLWithBookingMemo(t *testing.T) {
	res, err := testSePay().CreateQR(context.Background(), CreateQRRequest{BookingID: 42, AmountVND: 350000})
	if err != nil {
		t.Fatalf("CreateQR() error = %v", err)
	}
	if res.ProviderRef != "LAVERTE42" {
		t.Errorf("ProviderRef = %q, want LAVERTE42", res.ProviderRef)
	}
	for _, want := range []string{"https://vietqr.app/img?", "acc=0123456789", "bank=MSB", "amount=350000", "des=LAVERTE42"} {
		if !strings.Contains(res.QRContent, want) {
			t.Errorf("QRContent = %q, missing %q", res.QRContent, want)
		}
	}
}

func TestVerifyWebhookAcceptsCorrectlySignedBody(t *testing.T) {
	body := paidWebhookBody()
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	ev, err := testSePay().VerifyWebhook(context.Background(), body, headers)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if ev.ProviderRef != "LAVERTE42" {
		t.Errorf("ProviderRef = %q, want LAVERTE42", ev.ProviderRef)
	}
	if ev.ExternalRef != "998877" {
		t.Errorf("ExternalRef = %q, want 998877", ev.ExternalRef)
	}
	if ev.AmountVND != 350000 {
		t.Errorf("AmountVND = %d, want 350000", ev.AmountVND)
	}
	if !ev.Success {
		t.Error("Success = false, want true for transferType \"in\"")
	}
	if ev.Raw != string(body) {
		t.Error("Raw should carry the unmodified body")
	}
}

func TestVerifyWebhookRejectsWrongSecret(t *testing.T) {
	body := paidWebhookBody()
	headers := signedHeaders(body, "attacker-secret", time.Now())

	if _, err := testSePay().VerifyWebhook(context.Background(), body, headers); err == nil {
		t.Fatal("VerifyWebhook() with a body signed by the wrong secret = nil error, want error")
	}
}

func TestVerifyWebhookRejectsStaleTimestamp(t *testing.T) {
	body := paidWebhookBody()
	headers := signedHeaders(body, testWebhookSecret, time.Now().Add(-sepayReplayWindow-time.Minute))

	if _, err := testSePay().VerifyWebhook(context.Background(), body, headers); err == nil {
		t.Fatal("VerifyWebhook() outside the replay window = nil error, want error")
	}
}

func TestVerifyWebhookReturnsPingForDashboardTest(t *testing.T) {
	body := []byte(`{"id":1,"accountNumber":"0000000001","code":"","content":"Giao dich thu nghiem","transferAmount":2000,"transferType":"in"}`)
	headers := signedHeaders(body, testWebhookSecret, time.Now())

	_, err := testSePay().VerifyWebhook(context.Background(), body, headers)
	if !errors.Is(err, ErrWebhookPing) {
		t.Fatalf("VerifyWebhook() error = %v, want ErrWebhookPing", err)
	}
}

func TestParseBookingMemoSurvivesStrippedPunctuation(t *testing.T) {
	if got := parseBookingMemo("LAVERTE", "CT DEN:LAVERTE42 chuyen tien"); got != "LAVERTE42" {
		t.Errorf("parseBookingMemo() = %q, want LAVERTE42", got)
	}
	if got := parseBookingMemo("LAVERTE", "ck laverte-42"); got != "LAVERTE42" {
		t.Errorf("parseBookingMemo() lowercase+dash = %q, want LAVERTE42", got)
	}
	if got := parseBookingMemo("LAVERTE", "chuyen tien khong co memo"); got != "" {
		t.Errorf("parseBookingMemo() with no memo = %q, want empty", got)
	}
	if got := parseBookingMemo("LAVERTE", "CT DEN:LAVERTE chuyen tien"); got != "" {
		t.Errorf("parseBookingMemo() with prefix but no digits = %q, want empty", got)
	}
}
```

- [ ] **Step 11: Run the SePay unit tests**

Run: `go test ./util/checkout/... -v`
Expected: PASS on all six tests — no network and no DB, so this runs in any environment.

- [ ] **Step 12: Build, vet, test, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add model/payment.go migrations/0006_payments.sql repository/payment util/checkout
git commit -m "feat: Payment model, once-only settlement guard, SePay VietQR client"
```

---

---

### Task 12: Guest booking creation usecase + public HTTP route with anti-spam

**Files:**
- Create: `payload/booking.go`
- Create: `presenter/booking.go`
- Create: `usecase/booking/interface.go`
- Create: `usecase/booking/usecase.go`
- Create: `usecase/booking/create.go`
- Create: `usecase/booking/create_test.go`
- Create: `delivery/http/booking/handler.go`
- Create: `delivery/http/booking/route.go`
- Modify: `delivery/http/http.go` — mount the PUBLIC `/bookings` group with per-IP + per-phone limiters
- Modify: `delivery/http/http_test.go` — stub the new usecase in `newTestServer()`
- Modify: `cmd/main.go` — wire booking/blocked-slot/payment repos, SePay provider, booking usecase

**Interfaces:**
- Consumes: `bookingrepo.IRepository` + `bookingrepo.ErrSlotConflict` (Task 9); `homerepo.IRepository` (Task 7); `blockedslotrepo.IRepository` (Task 10); `paymentrepo.IRepository` (Task 11); `pricinguc.IUseCase.Compute` (Task 8); `checkout.IPaymentProvider` + `checkout.CreateQRRequest`/`QRResult` (Task 11, constructed in `main` as `checkout.NewSePay(*cfg)` — the constructor takes `config.Config` by value); `jwtmw.RateLimitByIP`/`RateLimitByPhone` (Task 5); `cfg.BookingPendingTTLMinutes`, `cfg.RateLimitBookingPerMinIP`, `cfg.RateLimitBookingPerMinPhone` (Task 1).
- Produces: `payload.CreateBookingRequest`; `presenter.BookingResponse` + `presenter.ToBookingResponse(b *model.Booking, qrContent string) BookingResponse`.
- Produces: `bookinguc.IUseCase` — **declares only `Create` in this task; later tasks (guest booking lookup, admin walk-in/cancel/lock-code ops) add methods to this same interface, so do not treat it as closed.**
- Produces: `bookinguc.New(bookingRepo, homeRepo, blockedSlotRepo, paymentRepo, pricingUC, payment, cfg) IUseCase`.
- Produces: `bookinghttp.Init(g *echo.Group, uc bookinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, phoneLimit echo.MiddlewareFunc)` — mounts `POST /api/v1/bookings` with no JWT.

- [ ] **Step 1: Write `payload/booking.go`**

```go
package payload

import "time"

type CreateBookingRequest struct {
	HomeID        uint      `json:"home_id" validate:"required"`
	CustomerName  string    `json:"customer_name" validate:"required"`
	CustomerPhone string    `json:"customer_phone" validate:"required"`
	StartTime     time.Time `json:"start_time" validate:"required"`
	EndTime       time.Time `json:"end_time" validate:"required"`
	BookingType   string    `json:"booking_type" validate:"required,oneof=hourly overnight day"`
}
```

- [ ] **Step 2: Write `presenter/booking.go`**

```go
package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

type BookingResponse struct {
	ID            uint       `json:"id"`
	HomeID        uint       `json:"home_id"`
	CustomerName  string     `json:"customer_name"`
	CustomerPhone string     `json:"customer_phone"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       time.Time  `json:"end_time"`
	BookingType   string     `json:"booking_type"`
	ComputedPrice int64      `json:"computed_price"`
	Status        string     `json:"status"`
	ExpiresAt     *time.Time `json:"expires_at"`
	QRContent     string     `json:"qr_content"`
}

// ToBookingResponse takes qrContent separately: the QR lives on the Payment
// row, and a booking can be re-served with an already-issued QR.
func ToBookingResponse(b *model.Booking, qrContent string) BookingResponse {
	return BookingResponse{
		ID: b.ID, HomeID: b.HomeID, CustomerName: b.CustomerName, CustomerPhone: b.CustomerPhone,
		StartTime: b.StartTime, EndTime: b.EndTime, BookingType: b.BookingType,
		ComputedPrice: b.ComputedPrice, Status: b.Status, ExpiresAt: b.ExpiresAt, QRContent: qrContent,
	}
}
```

- [ ] **Step 3: Write `usecase/booking/interface.go`**

```go
package booking

import (
	"context"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

// IUseCase grows across later tasks (guest lookup, admin walk-in, cancel,
// lock-code) — add methods here instead of introducing a parallel interface.
type IUseCase interface {
	Create(ctx context.Context, req payload.CreateBookingRequest) (*presenter.BookingResponse, error)
}
```

- [ ] **Step 4: Write `usecase/booking/usecase.go`**

```go
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
```

- [ ] **Step 5: Write `usecase/booking/create.go`**

```go
package booking

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

func (uc *UseCase) Create(ctx context.Context, req payload.CreateBookingRequest) (*presenter.BookingResponse, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.Validation("end_time phai sau start_time")
	}
	if !model.IsValidBookingType(req.BookingType) {
		return nil, apperr.Validation("booking_type phai la 'hourly', 'overnight' hoac 'day'")
	}

	// Re-use check runs before pricing, slot checks and any provider call: a
	// phone still holding an un-expired pending booking gets its existing QR
	// back, so one caller cannot mint an unbounded pile of unpaid QR codes.
	existing, err := uc.bookingRepo.GetPendingByPhone(ctx, req.CustomerPhone)
	switch {
	case err == nil:
		pay, payErr := uc.paymentRepo.GetByBookingID(ctx, existing.ID)
		if payErr != nil {
			return nil, apperr.Internal(payErr)
		}
		resp := presenter.ToBookingResponse(existing, pay.QRContent)
		return &resp, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, apperr.Internal(err)
	}

	home, err := uc.homeRepo.GetByID(ctx, req.HomeID)
	if err != nil {
		return nil, apperr.NotFound(err)
	}
	if !home.IsActive {
		return nil, apperr.Validation("home dang tam ngung nhan khach")
	}

	blocked, err := uc.blockedSlotRepo.HasOverlap(ctx, req.HomeID, req.StartTime, req.EndTime)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if blocked {
		return nil, apperr.SlotConflict(nil)
	}

	now := time.Now()
	price, err := uc.pricingUC.Compute(ctx, home.Category, req.BookingType, req.StartTime, req.EndTime, now)
	if err != nil {
		return nil, err // already an apperr from usecase/pricing
	}

	expiresAt := now.Add(time.Duration(uc.cfg.BookingPendingTTLMinutes) * time.Minute)
	b := &model.Booking{
		HomeID:        req.HomeID,
		CustomerName:  req.CustomerName,
		CustomerPhone: req.CustomerPhone,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		BookingType:   req.BookingType,
		ComputedPrice: price,
		Status:        model.BookingStatusPendingPayment,
		ExpiresAt:     &expiresAt,
	}
	if err := uc.bookingRepo.Create(ctx, b); err != nil {
		// The DB exclusion constraint, not app code, decides who wins a race
		// for the same slot.
		if errors.Is(err, bookingrepo.ErrSlotConflict) {
			return nil, apperr.SlotConflict(err)
		}
		return nil, apperr.Internal(err)
	}

	qr, err := uc.payment.CreateQR(ctx, checkout.CreateQRRequest{BookingID: b.ID, AmountVND: price})
	if err != nil {
		return nil, apperr.Internal(err)
	}

	pay := &model.Payment{
		BookingID: b.ID,
		Provider:  model.PaymentProviderSePay,
		Amount:    price,
		Status:    model.PaymentStatusPending,
		QRContent: qr.QRContent,
	}
	if err := uc.paymentRepo.Create(ctx, pay); err != nil {
		return nil, apperr.Internal(err)
	}

	b.PaymentID = &pay.ID
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		return nil, apperr.Internal(err)
	}

	resp := presenter.ToBookingResponse(b, qr.QRContent)
	return &resp, nil
}
```

- [ ] **Step 6: Write `usecase/booking/create_test.go`** (hand-written fakes, no mock library, no DB)

```go
package booking

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

type fakeBookingRepo struct {
	pending     *model.Booking
	createErr   error
	created     []*model.Booking
	updateCalls int
	nextID      uint
}

func (f *fakeBookingRepo) Create(_ context.Context, b *model.Booking) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.nextID++
	b.ID = f.nextID
	f.created = append(f.created, b)
	return nil
}

func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	for _, b := range f.created {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBookingRepo) Update(_ context.Context, _ *model.Booking) error {
	f.updateCalls++
	return nil
}

func (f *fakeBookingRepo) GetPendingByPhone(_ context.Context, phone string) (*model.Booking, error) {
	if f.pending != nil && f.pending.CustomerPhone == phone {
		return f.pending, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBookingRepo) ListExpiredPending(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

func (f *fakeBookingRepo) ListByHomeAndDate(context.Context, uint, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

func (f *fakeBookingRepo) ListUpcomingMissingLockCode(context.Context, time.Time, time.Duration) ([]*model.Booking, error) {
	return nil, nil
}

func (f *fakeBookingRepo) ListReadyToSendLockCode(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

// CountConfirmedBetween joins bookingrepo.IRepository in Task 18. Stubbed here
// already so that task never has to retro-edit this file.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type fakeHomeRepo struct{ home *model.Home }

func (f *fakeHomeRepo) Create(context.Context, *model.Home) error { return nil }
func (f *fakeHomeRepo) Update(context.Context, *model.Home) error { return nil }
func (f *fakeHomeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	if f.home == nil || f.home.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return f.home, nil
}
func (f *fakeHomeRepo) List(context.Context) ([]*model.Home, error) { return nil, nil }

type fakeBlockedSlotRepo struct{ overlap bool }

func (f *fakeBlockedSlotRepo) Create(context.Context, *model.BlockedSlot) error { return nil }
func (f *fakeBlockedSlotRepo) Delete(context.Context, uint) error               { return nil }
func (f *fakeBlockedSlotRepo) ListByHome(context.Context, uint) ([]*model.BlockedSlot, error) {
	return nil, nil
}
func (f *fakeBlockedSlotRepo) HasOverlap(context.Context, uint, time.Time, time.Time) (bool, error) {
	return f.overlap, nil
}

type fakePaymentRepo struct {
	byBookingID map[uint]*model.Payment
	nextID      uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{byBookingID: map[uint]*model.Payment{}}
}

func (f *fakePaymentRepo) Create(_ context.Context, p *model.Payment) error {
	f.nextID++
	p.ID = f.nextID
	f.byBookingID[p.BookingID] = p
	return nil
}

func (f *fakePaymentRepo) GetByID(_ context.Context, id uint) (*model.Payment, error) {
	for _, p := range f.byBookingID {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakePaymentRepo) GetByBookingID(_ context.Context, bookingID uint) (*model.Payment, error) {
	p, ok := f.byBookingID[bookingID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return p, nil
}

func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return true, nil
}

func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

// MarkExpiredIfPending joins paymentrepo.IRepository in Task 17, stubbed here
// for the same reason as fakeBookingRepo.CountConfirmedBetween.
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }

type fakePricingUC struct {
	price int64
	calls int
}

func (f *fakePricingUC) Compute(context.Context, string, string, time.Time, time.Time, time.Time) (int64, error) {
	f.calls++
	return f.price, nil
}

type fakePaymentProvider struct {
	qrContent string
	calls     int
}

func (f *fakePaymentProvider) CreateQR(_ context.Context, req checkout.CreateQRRequest) (*checkout.QRResult, error) {
	f.calls++
	return &checkout.QRResult{
		QRContent:   f.qrContent,
		ProviderRef: fmt.Sprintf("LAVERTE%d", req.BookingID),
	}, nil
}

func (f *fakePaymentProvider) VerifyWebhook(context.Context, []byte, http.Header) (*checkout.WebhookEvent, error) {
	return nil, nil
}

type harness struct {
	uc       IUseCase
	bookings *fakeBookingRepo
	homes    *fakeHomeRepo
	slots    *fakeBlockedSlotRepo
	payments *fakePaymentRepo
	pricing  *fakePricingUC
	provider *fakePaymentProvider
}

func newHarness() *harness {
	h := &harness{
		bookings: &fakeBookingRepo{},
		homes:    &fakeHomeRepo{home: &model.Home{ID: 1, Name: "Nest 1", Category: model.HomeCategoryNest, IsActive: true}},
		slots:    &fakeBlockedSlotRepo{},
		payments: newFakePaymentRepo(),
		pricing:  &fakePricingUC{price: 300000},
		provider: &fakePaymentProvider{qrContent: "00020101021238..."},
	}
	h.uc = New(h.bookings, h.homes, h.slots, h.payments, h.pricing, h.provider, config.Config{BookingPendingTTLMinutes: 15})
	return h
}

func validRequest() payload.CreateBookingRequest {
	start := time.Now().Add(2 * time.Hour)
	return payload.CreateBookingRequest{
		HomeID: 1, CustomerName: "Khach A", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(3 * time.Hour), BookingType: model.BookingTypeHourly,
	}
}

func TestCreateHappyPathReturnsPendingPaymentWithQR(t *testing.T) {
	h := newHarness()

	resp, err := h.uc.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if resp.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want %q", resp.Status, model.BookingStatusPendingPayment)
	}
	if resp.ComputedPrice != 300000 {
		t.Errorf("ComputedPrice = %d, want 300000", resp.ComputedPrice)
	}
	if resp.QRContent != "00020101021238..." {
		t.Errorf("QRContent = %q, want the provider's QR", resp.QRContent)
	}
	if resp.ExpiresAt == nil {
		t.Error("ExpiresAt = nil, want the pending TTL deadline")
	}
	pay, err := h.payments.GetByBookingID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("payment for booking %d not created: %v", resp.ID, err)
	}
	if pay.Status != model.PaymentStatusPending || pay.Amount != 300000 {
		t.Errorf("payment = %+v, want pending/300000", pay)
	}
	if h.bookings.updateCalls != 1 {
		t.Errorf("bookingRepo.Update calls = %d, want 1 (link payment_id)", h.bookings.updateCalls)
	}
}

func TestCreateReturnsExistingPendingBookingForSamePhone(t *testing.T) {
	h := newHarness()
	expires := time.Now().Add(10 * time.Minute)
	h.bookings.pending = &model.Booking{
		ID: 42, HomeID: 1, CustomerName: "Khach A", CustomerPhone: "0900000001",
		BookingType: model.BookingTypeHourly, ComputedPrice: 300000,
		Status: model.BookingStatusPendingPayment, ExpiresAt: &expires,
	}
	h.payments.byBookingID[42] = &model.Payment{
		ID: 7, BookingID: 42, Provider: model.PaymentProviderSePay, Amount: 300000,
		Status: model.PaymentStatusPending, QRContent: "QR-EXISTING",
	}

	resp, err := h.uc.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("ID = %d, want the existing pending booking 42", resp.ID)
	}
	if resp.QRContent != "QR-EXISTING" {
		t.Errorf("QRContent = %q, want QR-EXISTING", resp.QRContent)
	}
	if h.pricing.calls != 0 {
		t.Errorf("pricing Compute calls = %d, want 0 — re-use must short-circuit before any work", h.pricing.calls)
	}
	if h.provider.calls != 0 {
		t.Errorf("CreateQR calls = %d, want 0 — no second QR for the same phone", h.provider.calls)
	}
	if len(h.bookings.created) != 0 {
		t.Errorf("created %d bookings, want 0", len(h.bookings.created))
	}
}

func TestCreateRejectsBlockedSlot(t *testing.T) {
	h := newHarness()
	h.slots.overlap = true

	_, err := h.uc.Create(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Create() over a blocked slot = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeSlotConflict {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeSlotConflict)
	}
	if h.provider.calls != 0 {
		t.Errorf("CreateQR calls = %d, want 0", h.provider.calls)
	}
}

func TestCreateMapsRepoSlotConflictToApperr(t *testing.T) {
	h := newHarness()
	h.bookings.createErr = bookingrepo.ErrSlotConflict

	_, err := h.uc.Create(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Create() with ErrSlotConflict = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("apperr.As(%v) = false, want true", err)
	}
	if e.Code != apperr.CodeSlotConflict {
		t.Errorf("Code = %q, want %q", e.Code, apperr.CodeSlotConflict)
	}
}

func TestCreateRejectsInactiveHome(t *testing.T) {
	h := newHarness()
	h.homes.home.IsActive = false

	if _, err := h.uc.Create(context.Background(), validRequest()); err == nil {
		t.Fatal("Create() for an inactive home = nil error, want error")
	}
	if len(h.bookings.created) != 0 {
		t.Errorf("created %d bookings, want 0", len(h.bookings.created))
	}
}
```

- [ ] **Step 7: Run the booking usecase tests**

Run: `go test ./usecase/booking/... -v`
Expected: PASS — all five tests (`TestCreateHappyPathReturnsPendingPaymentWithQR`, `TestCreateReturnsExistingPendingBookingForSamePhone`, `TestCreateRejectsBlockedSlot`, `TestCreateMapsRepoSlotConflictToApperr`, `TestCreateRejectsInactiveHome`).

- [ ] **Step 8: Write `delivery/http/booking/handler.go`**

```go
package booking

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        bookinguc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc bookinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) create(c echo.Context) error {
	// bindBookingRequest already consumed the body; re-binding here would read
	// an empty reader.
	req, ok := c.Get("booking_request").(payload.CreateBookingRequest)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	resp, err := h.uc.Create(c.Request().Context(), req)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
```

- [ ] **Step 9: Write `delivery/http/booking/route.go`**

```go
package booking

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/payload"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
)

// bindBookingRequest parses the body and publishes customer_phone into the
// context. Echo runs every middleware before the handler, so the per-phone
// limiter can only see a phone that something earlier in the chain put there;
// the parsed struct rides along so the handler never re-reads the body.
func bindBookingRequest(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req payload.CreateBookingRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		c.Set("booking_request", req)
		c.Set("customer_phone", req.CustomerPhone)
		return next(c)
	}
}

// Init mounts the public guest-booking route. Route middleware runs in the
// given order: bind first, then the per-phone limiter, then the handler.
func Init(g *echo.Group, uc bookinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc, phoneLimit echo.MiddlewareFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.POST("", h.create, bindBookingRequest, phoneLimit)
}
```

- [ ] **Step 10: Modify `delivery/http/http.go`** — add the booking usecase parameter, the two limiters and the PUBLIC group

Add to the import block:

```go
	bookinghttp "github.com/johnquangdev/laverte-home/delivery/http/booking"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
```

Add one field to the `Deps` struct (Task 6):

```go
	BookingUC bookinguc.IUseCase
```

Insert immediately after the existing `authhttp.Init(api.Group("/auth", ipLimit), authUC, handleErr, handleOK)` line:

```go
	// Guests book without an account: this group is deliberately outside the
	// JWT group, protected only by the two rate limiters.
	bookingIPLimit := jwtmw.RateLimitByIP("booking", deps.Limiter, cfg.RateLimitBookingPerMinIP, window)
	bookingPhoneLimit := jwtmw.RateLimitByPhone("booking", deps.Limiter, cfg.RateLimitBookingPerMinPhone, window)
	bookinghttp.Init(api.Group("/bookings", bookingIPLimit), deps.BookingUC, handleErr, handleOK, bookingPhoneLimit)
```

- [ ] **Step 11: Modify `delivery/http/http_test.go`** — stub the new usecase

Add the stub:

```go
type stubBookingUC struct{}

func (stubBookingUC) Create(context.Context, payload.CreateBookingRequest) (*presenter.BookingResponse, error) {
	return nil, nil
}
```

and add `BookingUC: stubBookingUC{},` to the `Deps` literal inside `newTestServer()`.

- [ ] **Step 12: Modify `cmd/main.go`** — wire the new repos, the SePay provider and the booking usecase

Add to the import block:

```go
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	bookinguc "github.com/johnquangdev/laverte-home/usecase/booking"
	"github.com/johnquangdev/laverte-home/util/checkout"
```

Add after the existing repo/usecase construction (`homes`, `pricingRules`, `pricingUC` already exist from Tasks 7-8):

```go
	bookings := bookingrepo.NewPG(dbFactory)
	blockedSlots := blockedslotrepo.NewPG(dbFactory)
	payments := paymentrepo.NewPG(dbFactory)

	sepay := checkout.NewSePay(*cfg)
	bookingUC := bookinguc.New(bookings, homes, blockedSlots, payments, pricingUC, sepay, *cfg)
```

and add `BookingUC: bookingUC,` to the `httpserver.Deps{...}` literal.

- [ ] **Step 13: Build, vet, test, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add payload/booking.go presenter/booking.go usecase/booking delivery/http/booking delivery/http/http.go delivery/http/http_test.go cmd/main.go
git commit -m "feat: guest booking creation with QR re-use anti-spam and public rate-limited route"
```

---

### Task 13: SePay webhook — settle payment, confirm booking

**Files:**
- Create: `util/notify/interface.go`
- Create: `util/notify/noop.go`
- Create: `util/gcalendar/interface.go`
- Create: `util/gcalendar/noop.go`
- Create: `usecase/billing/interface.go`
- Create: `usecase/billing/usecase.go`
- Create: `usecase/billing/webhook.go`
- Create: `usecase/billing/webhook_test.go`
- Create: `delivery/http/webhook/handler.go`
- Create: `delivery/http/webhook/route.go`
- Modify: `delivery/http/http.go` — mount the PUBLIC `/webhooks` group
- Modify: `delivery/http/http_test.go` — stub the billing usecase in `newTestServer()`
- Modify: `cmd/main.go` — wire the billing usecase with the no-op notifier/calendar

**Interfaces:**
- Consumes: `checkout.IPaymentProvider.VerifyWebhook` + `checkout.WebhookEvent` + `checkout.ErrWebhookPing` (Task 11); `bookingrepo.IRepository` (Task 9); `paymentrepo.IRepository` (Task 11); `homerepo.IRepository` (Task 7); `cfg.SePayTransferPrefix`, `cfg.RateLimitWebhookPerMin` (Task 1).
- Produces: `notify.INotifier` (`BookingConfirmed`, `LockCode`, `AdminLockCodeMissing`) + `notify.NewNoop()` — declared here as consumer-side ports; **Task 15 adds the real ZNS/email adapter behind the same interface.**
- Produces: `gcalendar.ICalendar` (`CreateEvent`, `DeleteEvent`) + `gcalendar.NewNoop()` — **Task 14 adds the real service-account adapter behind the same interface.**
- Produces: `billinguc.IUseCase` with `HandleSePayWebhook(ctx context.Context, raw []byte, headers http.Header) error`; `billinguc.New(bookingRepo, paymentRepo, payment, notifier, calendar, homeRepo, log, cfg) IUseCase`.
- Produces: `webhookhttp.Init(g *echo.Group, uc billinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc)` — mounts `POST /api/v1/webhooks/sepay` with no JWT.

- [ ] **Step 1: Write `util/notify/interface.go`**

```go
package notify

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

// INotifier is declared on the consumer side so usecase/billing compiles and
// is testable now; the Zalo ZNS + admin-email adapter lands in Task 15.
type INotifier interface {
	BookingConfirmed(ctx context.Context, b *model.Booking) error
	LockCode(ctx context.Context, b *model.Booking, code string) error
	AdminLockCodeMissing(ctx context.Context, b *model.Booking) error
}
```

- [ ] **Step 2: Write `util/notify/noop.go`**

```go
package notify

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

type noopNotifier struct{}

// NewNoop keeps main bootable while notifications are unimplemented — Task 15
// swaps it for the real adapter.
func NewNoop() INotifier { return noopNotifier{} }

func (noopNotifier) BookingConfirmed(context.Context, *model.Booking) error     { return nil }
func (noopNotifier) LockCode(context.Context, *model.Booking, string) error     { return nil }
func (noopNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error { return nil }
```

- [ ] **Step 3: Write `util/gcalendar/interface.go`**

```go
package gcalendar

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

// ICalendar is declared on the consumer side so usecase/billing compiles and
// is testable now; the Google service-account adapter lands in Task 14.
type ICalendar interface {
	CreateEvent(ctx context.Context, calendarID string, b *model.Booking) (string, error)
	DeleteEvent(ctx context.Context, calendarID, eventID string) error
}
```

- [ ] **Step 4: Write `util/gcalendar/noop.go`**

```go
package gcalendar

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

type noopCalendar struct{}

// NewNoop keeps main bootable while calendar push is unimplemented — Task 14
// swaps it for the real adapter.
func NewNoop() ICalendar { return noopCalendar{} }

func (noopCalendar) CreateEvent(context.Context, string, *model.Booking) (string, error) {
	return "", nil
}

func (noopCalendar) DeleteEvent(context.Context, string, string) error { return nil }
```

- [ ] **Step 5: Write `usecase/billing/interface.go`**

```go
package billing

import (
	"context"
	"net/http"
)

type IUseCase interface {
	// HandleSePayWebhook takes the raw request body and headers because the
	// provider signs the exact bytes it sent.
	HandleSePayWebhook(ctx context.Context, raw []byte, headers http.Header) error
}
```

- [ ] **Step 6: Write `usecase/billing/usecase.go`**

```go
package billing

import (
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	"github.com/johnquangdev/laverte-home/util/checkout"
	"github.com/johnquangdev/laverte-home/util/gcalendar"
	"github.com/johnquangdev/laverte-home/util/notify"
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
```

- [ ] **Step 7: Write `usecase/billing/webhook.go`**

```go
package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

func (uc *UseCase) HandleSePayWebhook(ctx context.Context, raw []byte, headers http.Header) error {
	event, err := uc.payment.VerifyWebhook(ctx, raw, headers)
	if err != nil {
		// The provider dashboard probes this endpoint with a body that carries
		// no transaction; answering 200 keeps the webhook marked healthy.
		if errors.Is(err, checkout.ErrWebhookPing) {
			return nil
		}
		return apperr.Unauthorized(err)
	}

	// Outbound/debit transfers hit the same endpoint; only money coming in
	// settles a booking.
	if !event.Success {
		return nil
	}

	bookingID, err := bookingIDFromMemo(uc.cfg.SePayTransferPrefix, event.ProviderRef)
	if err != nil {
		return apperr.Validation("khong doc duoc booking id tu noi dung chuyen khoan")
	}

	booking, err := uc.bookingRepo.GetByID(ctx, bookingID)
	if err != nil {
		return apperr.NotFound(err)
	}

	// The provider redelivers until it gets a 200, so an already-confirmed
	// booking is a success, not an error.
	if booking.Status == model.BookingStatusConfirmed {
		return nil
	}
	if booking.Status != model.BookingStatusPendingPayment {
		return apperr.BookingExpired(nil)
	}
	if event.AmountVND != booking.ComputedPrice {
		return apperr.AmountMismatch(nil)
	}

	payment, err := uc.paymentRepo.GetByBookingID(ctx, booking.ID)
	if err != nil {
		return apperr.NotFound(err)
	}
	marked, err := uc.paymentRepo.MarkPaidIfPending(ctx, payment.ID, event.ExternalRef, time.Now())
	if err != nil {
		return apperr.Internal(err)
	}
	// Lost the race to a concurrent delivery that already settled this payment.
	if !marked {
		return nil
	}

	booking.Status = model.BookingStatusConfirmed
	if err := uc.bookingRepo.Update(ctx, booking); err != nil {
		return apperr.Internal(err)
	}

	// Everything below is best-effort: the money has moved and the booking is
	// confirmed, so a side-effect failure must not make the provider retry a
	// webhook that was already fully applied.
	uc.pushCalendarEvent(ctx, booking)
	if err := uc.notifier.BookingConfirmed(ctx, booking); err != nil {
		uc.log.Error("webhook: booking-confirmed notification failed",
			zap.Uint("booking_id", booking.ID), zap.Error(err))
	}

	return nil
}

func (uc *UseCase) pushCalendarEvent(ctx context.Context, b *model.Booking) {
	home, err := uc.homeRepo.GetByID(ctx, b.HomeID)
	if err != nil {
		uc.log.Error("webhook: load home for calendar push failed",
			zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	if home.GoogleCalendarID == "" {
		return
	}

	eventID, err := uc.calendar.CreateEvent(ctx, home.GoogleCalendarID, b)
	if err != nil {
		uc.log.Error("webhook: calendar event create failed",
			zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}

	b.GoogleCalendarEventID = eventID
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		uc.log.Error("webhook: persist calendar event id failed",
			zap.Uint("booking_id", b.ID), zap.String("event_id", eventID), zap.Error(err))
	}
}

// bookingIDFromMemo recovers the booking id from a transfer memo shaped
// "<prefix><id>". Banks uppercase memos and prepend their own noise, so the
// prefix is matched case-insensitively anywhere in the string and only the
// digits immediately following it are read.
func bookingIDFromMemo(prefix, memo string) (uint, error) {
	if prefix == "" {
		return 0, errors.New("billing: empty transfer prefix")
	}
	upper := strings.ToUpper(strings.TrimSpace(memo))
	idx := strings.Index(upper, strings.ToUpper(prefix))
	if idx < 0 {
		return 0, fmt.Errorf("billing: memo %q has no prefix %q", memo, prefix)
	}

	rest := upper[idx+len(prefix):]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, fmt.Errorf("billing: memo %q has no booking id after prefix %q", memo, prefix)
	}

	id, err := strconv.ParseUint(rest[:digits], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("billing: parse booking id from memo %q: %w", memo, err)
	}
	return uint(id), nil
}
```

- [ ] **Step 8: Write `usecase/billing/webhook_test.go`** (fakes for every port, no DB)

```go
package billing

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/util/checkout"
)

type fakeBookingRepo struct {
	booking     *model.Booking
	updateCalls int
}

func (f *fakeBookingRepo) Create(context.Context, *model.Booking) error { return nil }

func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	if f.booking == nil || f.booking.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return f.booking, nil
}

func (f *fakeBookingRepo) Update(_ context.Context, b *model.Booking) error {
	f.updateCalls++
	f.booking = b
	return nil
}

func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBookingRepo) ListExpiredPending(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

func (f *fakeBookingRepo) ListByHomeAndDate(context.Context, uint, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

func (f *fakeBookingRepo) ListUpcomingMissingLockCode(context.Context, time.Time, time.Duration) ([]*model.Booking, error) {
	return nil, nil
}

func (f *fakeBookingRepo) ListReadyToSendLockCode(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

// CountConfirmedBetween joins bookingrepo.IRepository in Task 18. Stubbed here
// already so that task never has to retro-edit this file.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type fakePaymentRepo struct {
	payment   *model.Payment
	markOK    bool
	markCalls int
}

func (f *fakePaymentRepo) Create(context.Context, *model.Payment) error { return nil }

func (f *fakePaymentRepo) GetByID(context.Context, uint) (*model.Payment, error) {
	if f.payment == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.payment, nil
}

func (f *fakePaymentRepo) GetByBookingID(context.Context, uint) (*model.Payment, error) {
	if f.payment == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.payment, nil
}

func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	f.markCalls++
	return f.markOK, nil
}

func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

// MarkExpiredIfPending joins paymentrepo.IRepository in Task 17, stubbed here
// for the same reason as fakeBookingRepo.CountConfirmedBetween.
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }

type fakeHomeRepo struct{ home *model.Home }

func (f *fakeHomeRepo) Create(context.Context, *model.Home) error { return nil }
func (f *fakeHomeRepo) Update(context.Context, *model.Home) error { return nil }
func (f *fakeHomeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	if f.home == nil || f.home.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return f.home, nil
}
func (f *fakeHomeRepo) List(context.Context) ([]*model.Home, error) { return nil, nil }

type fakeProvider struct {
	event     *checkout.WebhookEvent
	verifyErr error
}

func (f *fakeProvider) CreateQR(context.Context, checkout.CreateQRRequest) (*checkout.QRResult, error) {
	return nil, nil
}

func (f *fakeProvider) VerifyWebhook(context.Context, []byte, http.Header) (*checkout.WebhookEvent, error) {
	return f.event, f.verifyErr
}

type fakeNotifier struct{ confirmedCalls int }

func (f *fakeNotifier) BookingConfirmed(context.Context, *model.Booking) error {
	f.confirmedCalls++
	return nil
}
func (f *fakeNotifier) LockCode(context.Context, *model.Booking, string) error     { return nil }
func (f *fakeNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error { return nil }

type fakeCalendar struct {
	eventID     string
	createErr   error
	createCalls int
}

func (f *fakeCalendar) CreateEvent(context.Context, string, *model.Booking) (string, error) {
	f.createCalls++
	return f.eventID, f.createErr
}

func (f *fakeCalendar) DeleteEvent(context.Context, string, string) error { return nil }

type harness struct {
	uc       IUseCase
	bookings *fakeBookingRepo
	payments *fakePaymentRepo
	homes    *fakeHomeRepo
	provider *fakeProvider
	notifier *fakeNotifier
	calendar *fakeCalendar
}

func newHarness(booking *model.Booking, event *checkout.WebhookEvent, verifyErr error) *harness {
	h := &harness{
		bookings: &fakeBookingRepo{booking: booking},
		payments: &fakePaymentRepo{
			payment: &model.Payment{
				ID: 77, BookingID: booking.ID, Provider: model.PaymentProviderSePay,
				Amount: booking.ComputedPrice, Status: model.PaymentStatusPending, QRContent: "QR",
			},
			markOK: true,
		},
		homes:    &fakeHomeRepo{home: &model.Home{ID: booking.HomeID, Name: "Nest 1", Category: model.HomeCategoryNest, GoogleCalendarID: "cal-1", IsActive: true}},
		provider: &fakeProvider{event: event, verifyErr: verifyErr},
		notifier: &fakeNotifier{},
		calendar: &fakeCalendar{eventID: "gcal-evt-1"},
	}
	h.uc = New(h.bookings, h.payments, h.provider, h.notifier, h.calendar, h.homes,
		zap.NewNop(), config.Config{SePayTransferPrefix: "LAVERTE"})
	return h
}

func pendingBooking() *model.Booking {
	return &model.Booking{
		ID: 42, HomeID: 1, CustomerName: "Khach A", CustomerPhone: "0900000001",
		BookingType: model.BookingTypeHourly, ComputedPrice: 300000,
		Status: model.BookingStatusPendingPayment,
	}
}

func paidEvent() *checkout.WebhookEvent {
	return &checkout.WebhookEvent{
		ProviderRef: "CT DEN:LAVERTE42", ExternalRef: "TXN-9001",
		Success: true, AmountVND: 300000, Raw: "{}",
	}
}

func TestHandleSePayWebhookConfirmsBooking(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v", err)
	}
	if h.payments.markCalls != 1 {
		t.Errorf("MarkPaidIfPending calls = %d, want 1", h.payments.markCalls)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", h.bookings.booking.Status, model.BookingStatusConfirmed)
	}
	if h.notifier.confirmedCalls != 1 {
		t.Errorf("BookingConfirmed calls = %d, want 1", h.notifier.confirmedCalls)
	}
	if h.bookings.booking.GoogleCalendarEventID != "gcal-evt-1" {
		t.Errorf("GoogleCalendarEventID = %q, want gcal-evt-1", h.bookings.booking.GoogleCalendarEventID)
	}
}

func TestHandleSePayWebhookAcknowledgesPing(t *testing.T) {
	h := newHarness(pendingBooking(), nil, checkout.ErrWebhookPing)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte(""), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() on ping error = %v, want nil", err)
	}
	if h.payments.markCalls != 0 || h.bookings.updateCalls != 0 {
		t.Errorf("ping mutated state: markCalls=%d updateCalls=%d", h.payments.markCalls, h.bookings.updateCalls)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want unchanged %q", h.bookings.booking.Status, model.BookingStatusPendingPayment)
	}
}

func TestHandleSePayWebhookRejectsAmountMismatch(t *testing.T) {
	event := paidEvent()
	event.AmountVND = 100000
	h := newHarness(pendingBooking(), event, nil)

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	if err == nil {
		t.Fatal("HandleSePayWebhook() with wrong amount = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeAmountMismatch {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeAmountMismatch)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want it left pending", h.bookings.booking.Status)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0", h.payments.markCalls)
	}
}

func TestHandleSePayWebhookIsIdempotentForConfirmedBooking(t *testing.T) {
	booking := pendingBooking()
	booking.Status = model.BookingStatusConfirmed
	h := newHarness(booking, paidEvent(), nil)

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() on redelivery error = %v, want nil", err)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0 on redelivery", h.payments.markCalls)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0 on redelivery", h.notifier.confirmedCalls)
	}
}

func TestHandleSePayWebhookStopsWhenPaymentAlreadyPaid(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.payments.markOK = false

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusPendingPayment {
		t.Errorf("Status = %q, want it untouched when the payment was already settled", h.bookings.booking.Status)
	}
	if h.bookings.updateCalls != 0 {
		t.Errorf("bookingRepo.Update calls = %d, want 0", h.bookings.updateCalls)
	}
	if h.notifier.confirmedCalls != 0 {
		t.Errorf("BookingConfirmed calls = %d, want 0", h.notifier.confirmedCalls)
	}
}

func TestHandleSePayWebhookConfirmsDespiteCalendarFailure(t *testing.T) {
	h := newHarness(pendingBooking(), paidEvent(), nil)
	h.calendar.createErr = errors.New("calendar 503")

	if err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{}); err != nil {
		t.Fatalf("HandleSePayWebhook() with a failing calendar error = %v, want nil", err)
	}
	if h.bookings.booking.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", h.bookings.booking.Status, model.BookingStatusConfirmed)
	}
	if h.calendar.createCalls != 1 {
		t.Errorf("CreateEvent calls = %d, want 1", h.calendar.createCalls)
	}
	if h.bookings.booking.GoogleCalendarEventID != "" {
		t.Errorf("GoogleCalendarEventID = %q, want empty after a failed push", h.bookings.booking.GoogleCalendarEventID)
	}
	if h.notifier.confirmedCalls != 1 {
		t.Errorf("BookingConfirmed calls = %d, want 1 — notification must still run", h.notifier.confirmedCalls)
	}
}

func TestHandleSePayWebhookRejectsExpiredBooking(t *testing.T) {
	booking := pendingBooking()
	booking.Status = model.BookingStatusExpired
	h := newHarness(booking, paidEvent(), nil)

	err := h.uc.HandleSePayWebhook(context.Background(), []byte("{}"), http.Header{})
	if err == nil {
		t.Fatal("HandleSePayWebhook() on an expired booking = nil error, want error")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeBookingExpired {
		t.Errorf("error = %v, want apperr with Code %q", err, apperr.CodeBookingExpired)
	}
	if h.payments.markCalls != 0 {
		t.Errorf("MarkPaidIfPending calls = %d, want 0", h.payments.markCalls)
	}
}

func TestBookingIDFromMemo(t *testing.T) {
	cases := []struct {
		memo    string
		want    uint
		wantErr bool
	}{
		{memo: "LAVERTE42", want: 42},
		{memo: "ct dEn:laverte7 ND", want: 7},
		{memo: "LAVERTE", wantErr: true},
		{memo: "OTHER99", wantErr: true},
	}
	for _, tc := range cases {
		got, err := bookingIDFromMemo("LAVERTE", tc.memo)
		if tc.wantErr {
			if err == nil {
				t.Errorf("bookingIDFromMemo(%q) = %d, want error", tc.memo, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("bookingIDFromMemo(%q) error = %v", tc.memo, err)
			continue
		}
		if got != tc.want {
			t.Errorf("bookingIDFromMemo(%q) = %d, want %d", tc.memo, got, tc.want)
		}
	}
}
```

- [ ] **Step 9: Run the billing usecase tests**

Run: `go test ./usecase/billing/... -v`
Expected: PASS — all eight tests (`TestHandleSePayWebhookConfirmsBooking`, `...AcknowledgesPing`, `...RejectsAmountMismatch`, `...IsIdempotentForConfirmedBooking`, `...StopsWhenPaymentAlreadyPaid`, `...ConfirmsDespiteCalendarFailure`, `...RejectsExpiredBooking`, `TestBookingIDFromMemo`).

- [ ] **Step 10: Write `delivery/http/webhook/handler.go`**

```go
package webhook

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
)

type HandleErrFunc func(c echo.Context, err error) error
type HandleOKFunc func(c echo.Context, data any) error

type Handler struct {
	uc        billinguc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newHandler(uc billinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *Handler {
	return &Handler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *Handler) sepay(c echo.Context) error {
	// The HMAC covers the exact bytes the provider sent — binding into a struct
	// and re-marshaling changes key order and spacing, invalidating it.
	raw, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot read body"})
	}
	if err := h.uc.HandleSePayWebhook(c.Request().Context(), raw, c.Request().Header); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}
```

- [ ] **Step 11: Write `delivery/http/webhook/route.go`**

```go
package webhook

import (
	"github.com/labstack/echo/v4"

	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
)

// Init mounts the provider callback. It carries no JWT — the request is
// authenticated by the signature VerifyWebhook checks.
func Init(g *echo.Group, uc billinguc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newHandler(uc, handleErr, handleOK)
	g.POST("/sepay", h.sepay)
}
```

- [ ] **Step 12: Modify `delivery/http/http.go`** — add the billing usecase parameter and the PUBLIC `/webhooks` group

Add to the import block:

```go
	webhookhttp "github.com/johnquangdev/laverte-home/delivery/http/webhook"
	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
```

Add one field to the `Deps` struct (Task 6):

```go
	BillingUC billinguc.IUseCase
```

Insert immediately after the `bookinghttp.Init(...)` line from Task 12:

```go
	// The provider's own retries are the load here, so this limit is much
	// higher than the guest-facing ones.
	webhookLimit := jwtmw.RateLimitByIP("payment-webhook", deps.Limiter, cfg.RateLimitWebhookPerMin, window)
	webhookhttp.Init(api.Group("/webhooks", webhookLimit), deps.BillingUC, handleErr, handleOK)
```

- [ ] **Step 13: Modify `delivery/http/http_test.go`** — stub the billing usecase

Add the stub:

```go
type stubBillingUC struct{}

func (stubBillingUC) HandleSePayWebhook(context.Context, []byte, http.Header) error { return nil }
```

and add `BillingUC: stubBillingUC{},` to the `Deps` literal inside `newTestServer()`.

- [ ] **Step 14: Modify `cmd/main.go`** — wire the billing usecase with no-op side-effect adapters

Add to the import block:

```go
	billinguc "github.com/johnquangdev/laverte-home/usecase/billing"
	"github.com/johnquangdev/laverte-home/util/gcalendar"
	"github.com/johnquangdev/laverte-home/util/notify"
```

Add after `bookingUC := bookinguc.New(...)` from Task 12:

```go
	// Tasks 14 and 15 replace these with the Google Calendar service-account
	// client and the ZNS/email notifier.
	calendarSvc := gcalendar.NewNoop()
	notifier := notify.NewNoop()

	billingUC := billinguc.New(bookings, payments, sepay, notifier, calendarSvc, homes, log, *cfg)
```

and add `BillingUC: billingUC,` to the `httpserver.Deps{...}` literal.

- [ ] **Step 15: Build, vet, test, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add util/notify util/gcalendar usecase/billing delivery/http/webhook delivery/http/http.go delivery/http/http_test.go cmd/main.go
git commit -m "feat: SePay webhook settles payment and confirms booking with best-effort calendar/notify"
```

---

## Phase 3 — External integrations (Calendar, ZNS, email)

### Task 14: Google Calendar adapter (service account)

**Files:**
- Create: `util/gcalendar/google.go`
- Create: `util/gcalendar/google_test.go`
- Modify: `go.mod` / `go.sum` (add `google.golang.org/api`)
- Modify: `cmd/main.go` (attempt the real calendar, fall back to the noop)

**Interfaces:**
- Consumes: `gcalendar.ICalendar` and `gcalendar.NewNoop()` from `util/gcalendar/interface.go` + `util/gcalendar/noop.go` (Task 13) — both already declared, do not redeclare; `config.Config.GoogleCalendarCredentialsJSON`, `config.Config.GoogleCalendarTimeZone` (Task 1); `model.Booking` (Task 9).
- Produces: `gcalendar.NewGoogle(cfg *config.Config) (ICalendar, error)` — the real `events.insert` / `events.delete` implementation, and the package-private `buildEvent(cfg, b) *calendar.Event` that owns all event formatting.

Per design doc §5 the Calendar push is **best-effort**: a booking must still confirm when Google is down or the key is missing. That is why `NewGoogle` returns an error instead of panicking — `cmd/main.go` degrades to `NewNoop()` and logs, rather than refusing to boot.

- [ ] **Step 1: Add the Google Calendar API dependency**

```bash
cd /Users/gunnguyen/go/src/github.com/johnquangdev/laverte-home
go get google.golang.org/api/calendar/v3
go get golang.org/x/oauth2/google
```

`golang.org/x/oauth2` is already a direct dependency from Task 4's Google OAuth; the second command only makes the `google` subpackage explicit. `google.golang.org/api` is new and also brings in `google.golang.org/api/option` and `google.golang.org/api/googleapi`, both used below.

- [ ] **Step 2: Write `util/gcalendar/google.go`**

```go
package gcalendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

type googleCalendar struct {
	svc *calendar.Service
	cfg *config.Config
}

// NewGoogle authenticates with a service-account key so the backend can push
// events unattended — no consent screen, no user refresh token to expire. The
// error return (rather than a panic) is deliberate: the caller degrades to the
// noop implementation because Calendar push is best-effort and must never
// block booking confirmation.
func NewGoogle(cfg *config.Config) (ICalendar, error) {
	if strings.TrimSpace(cfg.GoogleCalendarCredentialsJSON) == "" {
		return nil, errors.New("gcalendar: GOOGLE_CALENDAR_CREDENTIALS_JSON is empty")
	}

	// context.Background rather than a request context: the service outlives
	// any single request and uses this context to refresh its access token.
	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, []byte(cfg.GoogleCalendarCredentialsJSON), calendar.CalendarScope)
	if err != nil {
		return nil, fmt.Errorf("gcalendar: parse service account credentials: %w", err)
	}

	svc, err := calendar.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("gcalendar: new calendar service: %w", err)
	}
	return &googleCalendar{svc: svc, cfg: cfg}, nil
}

func (g *googleCalendar) CreateEvent(ctx context.Context, calendarID string, b *model.Booking) (string, error) {
	if calendarID == "" {
		return "", fmt.Errorf("gcalendar: home of booking %d has no google_calendar_id", b.ID)
	}

	created, err := g.svc.Events.Insert(calendarID, buildEvent(g.cfg, b)).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gcalendar: insert event for booking %d: %w", b.ID, err)
	}
	return created.Id, nil
}

func (g *googleCalendar) DeleteEvent(ctx context.Context, calendarID, eventID string) error {
	if calendarID == "" || eventID == "" {
		return nil
	}

	err := g.svc.Events.Delete(calendarID, eventID).Context(ctx).Do()
	if err == nil {
		return nil
	}

	// 404/410 means the event is already gone — a second cancel, or an admin
	// who deleted it by hand. The desired end state is reached, so surfacing
	// this as a failure would make cancellation non-idempotent.
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && (apiErr.Code == http.StatusNotFound || apiErr.Code == http.StatusGone) {
		return nil
	}
	return fmt.Errorf("gcalendar: delete event %s: %w", eventID, err)
}

// buildEvent is split out of CreateEvent so the summary/description/timezone
// formatting can be asserted without a Google API round-trip.
func buildEvent(cfg *config.Config, b *model.Booking) *calendar.Event {
	return &calendar.Event{
		Summary: fmt.Sprintf("Booking #%d — %s (%s)", b.ID, b.CustomerName, b.CustomerPhone),
		Description: fmt.Sprintf("Loai: %s\nGia: %d VND\nSDT: %s",
			b.BookingType, b.ComputedPrice, b.CustomerPhone),
		Start: &calendar.EventDateTime{
			DateTime: b.StartTime.Format(time.RFC3339),
			TimeZone: cfg.GoogleCalendarTimeZone,
		},
		End: &calendar.EventDateTime{
			DateTime: b.EndTime.Format(time.RFC3339),
			TimeZone: cfg.GoogleCalendarTimeZone,
		},
	}
}
```

- [ ] **Step 3: Write `util/gcalendar/google_test.go`** (no network: constructor validation, the noop contract, and the pure event builder)

```go
package gcalendar

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

func TestNewGoogleRejectsEmptyCredentials(t *testing.T) {
	if _, err := NewGoogle(&config.Config{}); err == nil {
		t.Fatal("NewGoogle() with empty GoogleCalendarCredentialsJSON = nil error, want error")
	}
}

func TestNewGoogleRejectsMalformedCredentials(t *testing.T) {
	cfg := &config.Config{GoogleCalendarCredentialsJSON: `{"type": "service_account"`}
	if _, err := NewGoogle(cfg); err == nil {
		t.Fatal("NewGoogle() with malformed JSON = nil error, want error")
	}
}

// The noop must be a usable fallback so cmd/main.go boots with no credentials.
func TestNoopIsUsableWithoutCredentials(t *testing.T) {
	cal := NewNoop()
	ctx := context.Background()

	eventID, err := cal.CreateEvent(ctx, "cal-1", &model.Booking{ID: 7})
	if err != nil {
		t.Fatalf("noop CreateEvent() error = %v, want nil", err)
	}
	if eventID != "" {
		t.Errorf("noop CreateEvent() id = %q, want empty", eventID)
	}
	if err := cal.DeleteEvent(ctx, "cal-1", "evt-1"); err != nil {
		t.Errorf("noop DeleteEvent() error = %v, want nil", err)
	}
}

func TestBuildEvent(t *testing.T) {
	cfg := &config.Config{GoogleCalendarTimeZone: "Asia/Ho_Chi_Minh"}
	start := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	b := &model.Booking{
		ID:            42,
		HomeID:        7,
		CustomerName:  "Nguyen Van A",
		CustomerPhone: "0901234567",
		StartTime:     start,
		EndTime:       start.Add(3 * time.Hour),
		BookingType:   model.BookingTypeHourly,
		ComputedPrice: 450000,
	}

	ev := buildEvent(cfg, b)

	wantSummary := "Booking #42 — Nguyen Van A (0901234567)"
	if ev.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", ev.Summary, wantSummary)
	}
	if ev.Start.TimeZone != "Asia/Ho_Chi_Minh" {
		t.Errorf("Start.TimeZone = %q, want Asia/Ho_Chi_Minh", ev.Start.TimeZone)
	}
	if ev.End.TimeZone != "Asia/Ho_Chi_Minh" {
		t.Errorf("End.TimeZone = %q, want Asia/Ho_Chi_Minh", ev.End.TimeZone)
	}
	if ev.Start.DateTime != "2026-08-01T14:00:00Z" {
		t.Errorf("Start.DateTime = %q, want 2026-08-01T14:00:00Z", ev.Start.DateTime)
	}
	if ev.End.DateTime != "2026-08-01T17:00:00Z" {
		t.Errorf("End.DateTime = %q, want 2026-08-01T17:00:00Z", ev.End.DateTime)
	}
	if !strings.Contains(ev.Description, "0901234567") {
		t.Errorf("Description = %q, want it to contain the customer phone", ev.Description)
	}
	if !strings.Contains(ev.Description, "450000") {
		t.Errorf("Description = %q, want it to contain the computed price", ev.Description)
	}
	if !strings.Contains(ev.Description, model.BookingTypeHourly) {
		t.Errorf("Description = %q, want it to contain the booking type", ev.Description)
	}
}
```

- [ ] **Step 4: Run the calendar tests**

Run: `go test ./util/gcalendar/... -v`
Expected: PASS — 4 tests (`TestNewGoogleRejectsEmptyCredentials`, `TestNewGoogleRejectsMalformedCredentials`, `TestNoopIsUsableWithoutCredentials`, `TestBuildEvent`). No network call is made.

- [ ] **Step 5: Modify `cmd/main.go` — replace the `gcalendar.NewNoop()` wiring with a real attempt**

Replace the single line that Task 13 wrote in the dependency-wiring section:

```go
	calendarSvc := gcalendar.NewNoop()
```

with this block (`log` is the `*zap.Logger` already in scope):

```go
	// Calendar push is best-effort per design doc §5, so a missing or invalid
	// service-account key downgrades to the noop instead of aborting boot.
	var calendarSvc = gcalendar.NewNoop()
	if cal, err := gcalendar.NewGoogle(cfg); err != nil {
		log.Warn("google calendar disabled", zap.Error(err))
	} else {
		calendarSvc = cal
	}
```

and pass `calendarSvc` into the billing usecase where `gcalendar.NewNoop()` was previously passed inline — every other argument stays exactly as Task 13 wired it:

```go
	billingUC := billinguc.New(bookings, payments, sepay, notifier, calendarSvc, homes, log, *cfg)
```

- [ ] **Step 6: Build, vet, commit**

```bash
go build ./...
go vet ./...
git add go.mod go.sum util/gcalendar/google.go util/gcalendar/google_test.go cmd/main.go
git commit -m "feat: Google Calendar service-account adapter with noop fallback"
```

---

### Task 15: Zalo ZNS customer notifier + SMTP admin alerts

**Files:**
- Create: `util/notify/zns.go`
- Create: `util/notify/smtp_admin.go`
- Create: `util/notify/composite.go`
- Create: `util/notify/zns_test.go`
- Create: `util/notify/composite_test.go`
- Modify: `cmd/main.go` (compose the real notifier when fully configured)

**Interfaces:**
- Consumes: `notify.INotifier` and `notify.NewNoop()` from `util/notify/interface.go` + `util/notify/noop.go` (Task 13) — both already declared, do not redeclare; `config.Config.ZNS*`, `config.Config.SMTP*`, `config.Config.AdminAlertEmail` (Task 1); `model.Booking` (Task 9).
- Produces: `notify.NewZNS(cfg *config.Config, httpClient *http.Client) *ZNSNotifier` (customer channel), `notify.NewSMTPAdmin(cfg *config.Config) *SMTPAdminNotifier` (admin channel), `notify.NewComposite(customer, admin INotifier) INotifier`, and the package-private `normalizeVNPhone(phone string) string`.

Per design doc §6 there are exactly two channels: ZNS to the customer's phone (no app install, no login) and email to the admin. `NewComposite` keeps that split out of the billing usecase, which only ever sees one `INotifier`. No new module dependency — `net/smtp` is stdlib and `net/http` is already in use.

- [ ] **Step 1: Write `util/notify/zns.go`**

```go
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

// vnTimeLayout is the day-first format Vietnamese customers and admins read;
// also used by smtp_admin.go.
const vnTimeLayout = "02/01/2006 15:04"

// ZNSNotifier is the customer channel: Zalo delivers by phone number, so the
// guest needs neither an account on this system nor an app install.
type ZNSNotifier struct {
	endpoint            string
	accessToken         string
	confirmedTemplateID string
	lockCodeTemplateID  string
	httpClient          *http.Client
}

func NewZNS(cfg *config.Config, httpClient *http.Client) *ZNSNotifier {
	return &ZNSNotifier{
		endpoint:            cfg.ZNSEndpoint,
		accessToken:         cfg.ZNSAccessToken,
		confirmedTemplateID: cfg.ZNSBookingConfirmedTemplateID,
		lockCodeTemplateID:  cfg.ZNSLockCodeTemplateID,
		httpClient:          httpClient,
	}
}

type znsRequest struct {
	Phone        string            `json:"phone"`
	TemplateID   string            `json:"template_id"`
	TemplateData map[string]string `json:"template_data"`
}

type znsResponse struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

func (z *ZNSNotifier) BookingConfirmed(ctx context.Context, b *model.Booking) error {
	return z.send(ctx, b.CustomerPhone, z.confirmedTemplateID, map[string]string{
		"booking_id":    strconv.FormatUint(uint64(b.ID), 10),
		"customer_name": b.CustomerName,
		"start_time":    b.StartTime.Format(vnTimeLayout),
		"price":         strconv.FormatInt(b.ComputedPrice, 10),
	})
}

func (z *ZNSNotifier) LockCode(ctx context.Context, b *model.Booking, code string) error {
	return z.send(ctx, b.CustomerPhone, z.lockCodeTemplateID, map[string]string{
		"booking_id": strconv.FormatUint(uint64(b.ID), 10),
		"lock_code":  code,
		"start_time": b.StartTime.Format(vnTimeLayout),
	})
}

// AdminLockCodeMissing is intentionally inert: ZNS is the customer channel and
// the admin has no Zalo template. NewComposite routes this call to SMTP.
func (z *ZNSNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error { return nil }

func (z *ZNSNotifier) send(ctx context.Context, phone, templateID string, data map[string]string) error {
	if z.accessToken == "" || templateID == "" {
		return errors.New("notify/zns: access token or template id not configured")
	}

	body, err := json.Marshal(znsRequest{
		Phone:        normalizeVNPhone(phone),
		TemplateID:   templateID,
		TemplateData: data,
	})
	if err != nil {
		return fmt.Errorf("notify/zns: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, z.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify/zns: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", z.accessToken)

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify/zns: post %s: %w", z.endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out znsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("notify/zns: decode response: %w", err)
	}
	// ZNS answers HTTP 200 even for rejected sends and reports the real outcome
	// only in the body's error field, so checking the status code alone would
	// silently drop messages.
	if out.Error != 0 {
		return fmt.Errorf("notify/zns: send failed (%d): %s", out.Error, out.Message)
	}
	return nil
}

// normalizeVNPhone converts a Vietnamese number to the 84XXXXXXXXX form ZNS
// requires; Zalo rejects the local 0-prefixed and +84 spellings as invalid
// recipients. A leading 0 is checked before a leading 84 because 084 is itself
// a valid mobile prefix (084xxxxxxx -> 8484xxxxxxx).
func normalizeVNPhone(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}

	out := digits.String()
	switch {
	case strings.HasPrefix(out, "0"):
		return "84" + out[1:]
	default:
		return out
	}
}
```

- [ ] **Step 2: Write `util/notify/smtp_admin.go`**

```go
package notify

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

// SMTPAdminNotifier is the admin channel. It carries one message: a confirmed
// booking is about to start and nobody has entered the door lock code yet.
type SMTPAdminNotifier struct {
	host       string
	port       string
	username   string
	password   string
	from       string
	adminEmail string
}

func NewSMTPAdmin(cfg *config.Config) *SMTPAdminNotifier {
	return &SMTPAdminNotifier{
		host:       cfg.SMTPHost,
		port:       cfg.SMTPPort,
		username:   cfg.SMTPUsername,
		password:   cfg.SMTPPassword,
		from:       cfg.SMTPFrom,
		adminEmail: cfg.AdminAlertEmail,
	}
}

// BookingConfirmed and LockCode are intentionally inert: guests are reached
// over ZNS, and this type has no customer address to mail.
func (s *SMTPAdminNotifier) BookingConfirmed(context.Context, *model.Booking) error { return nil }

func (s *SMTPAdminNotifier) LockCode(context.Context, *model.Booking, string) error { return nil }

func (s *SMTPAdminNotifier) AdminLockCodeMissing(_ context.Context, b *model.Booking) error {
	if s.host == "" || s.adminEmail == "" {
		return errors.New("notify/smtp: SMTP host or admin alert email not configured")
	}

	subject := fmt.Sprintf("[laverte-home] Booking #%d chua co ma khoa cua", b.ID)
	body := fmt.Sprintf(
		"Booking #%d tai home %d bat dau luc %s va chua co ma khoa cua.\r\n"+
			"Khach: %s - %s\r\n"+
			"Vui long nhap ma khoa cua qua PATCH /api/v1/admin/bookings/%d/lock-code.\r\n",
		b.ID, b.HomeID, b.StartTime.Format(vnTimeLayout), b.CustomerName, b.CustomerPhone, b.ID)

	msg := "From: " + s.from + "\r\n" +
		"To: " + s.adminEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
		"\r\n" + body

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err := smtp.SendMail(addr, auth, s.from, []string{s.adminEmail}, []byte(msg)); err != nil {
		return fmt.Errorf("notify/smtp: send admin alert for booking %d: %w", b.ID, err)
	}
	return nil
}
```

- [ ] **Step 3: Write `util/notify/composite.go`**

```go
package notify

import (
	"context"

	"github.com/johnquangdev/laverte-home/model"
)

type composite struct {
	customer INotifier
	admin    INotifier
}

// NewComposite hides the two-channel split behind one INotifier so the billing
// usecase never learns that customers go over ZNS and admins over email.
func NewComposite(customer, admin INotifier) INotifier {
	return &composite{customer: customer, admin: admin}
}

func (c *composite) BookingConfirmed(ctx context.Context, b *model.Booking) error {
	return c.customer.BookingConfirmed(ctx, b)
}

func (c *composite) LockCode(ctx context.Context, b *model.Booking, code string) error {
	return c.customer.LockCode(ctx, b, code)
}

func (c *composite) AdminLockCodeMissing(ctx context.Context, b *model.Booking) error {
	return c.admin.AdminLockCodeMissing(ctx, b)
}
```

- [ ] **Step 4: Write `util/notify/zns_test.go`** (`httptest` server stands in for Zalo — no real network)

```go
package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

type capturedZNS struct {
	accessToken string
	body        znsRequest
}

// znsStub replies with responseBody and hands the captured request back over a
// channel, which gives the assertions a happens-before edge on the handler.
func znsStub(t *testing.T, responseBody string) (*httptest.Server, <-chan capturedZNS) {
	t.Helper()
	ch := make(chan capturedZNS, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got capturedZNS
		got.accessToken = r.Header.Get("access_token")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		ch <- got
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func znsConfig(endpoint string) *config.Config {
	return &config.Config{
		ZNSEndpoint:                   endpoint,
		ZNSAccessToken:                "tok-abc",
		ZNSBookingConfirmedTemplateID: "tpl-confirmed",
		ZNSLockCodeTemplateID:         "tpl-lock",
	}
}

func testBooking() *model.Booking {
	start := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	return &model.Booking{
		ID:            42,
		HomeID:        7,
		CustomerName:  "Nguyen Van A",
		CustomerPhone: "090 123 4567",
		StartTime:     start,
		EndTime:       start.Add(3 * time.Hour),
		BookingType:   model.BookingTypeHourly,
		ComputedPrice: 450000,
	}
}

func TestZNSBookingConfirmedPostsTemplateAndNormalizedPhone(t *testing.T) {
	srv, captured := znsStub(t, `{"error":0,"message":"Success"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	if err := n.BookingConfirmed(context.Background(), testBooking()); err != nil {
		t.Fatalf("BookingConfirmed() error = %v", err)
	}

	got := <-captured
	if got.accessToken != "tok-abc" {
		t.Errorf("access_token header = %q, want tok-abc", got.accessToken)
	}
	if got.body.TemplateID != "tpl-confirmed" {
		t.Errorf("template_id = %q, want tpl-confirmed", got.body.TemplateID)
	}
	if got.body.Phone != "84901234567" {
		t.Errorf("phone = %q, want 84901234567", got.body.Phone)
	}
	if got.body.TemplateData["booking_id"] != "42" {
		t.Errorf("template_data[booking_id] = %q, want 42", got.body.TemplateData["booking_id"])
	}
	if got.body.TemplateData["customer_name"] != "Nguyen Van A" {
		t.Errorf("template_data[customer_name] = %q", got.body.TemplateData["customer_name"])
	}
	if got.body.TemplateData["start_time"] != "01/08/2026 14:00" {
		t.Errorf("template_data[start_time] = %q, want 01/08/2026 14:00", got.body.TemplateData["start_time"])
	}
	if got.body.TemplateData["price"] != "450000" {
		t.Errorf("template_data[price] = %q, want 450000", got.body.TemplateData["price"])
	}
}

func TestZNSErrorFieldFailsDespiteHTTP200(t *testing.T) {
	srv, captured := znsStub(t, `{"error":1,"message":"bad template"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	err := n.BookingConfirmed(context.Background(), testBooking())
	if err == nil {
		t.Fatal("BookingConfirmed() = nil error, want error even though the status was 200")
	}
	if !strings.Contains(err.Error(), "bad template") {
		t.Errorf("error = %v, want it to carry the ZNS message", err)
	}
	<-captured
}

func TestZNSLockCodeSendsTheCode(t *testing.T) {
	srv, captured := znsStub(t, `{"error":0,"message":"Success"}`)
	n := NewZNS(znsConfig(srv.URL), srv.Client())

	if err := n.LockCode(context.Background(), testBooking(), "482913"); err != nil {
		t.Fatalf("LockCode() error = %v", err)
	}

	got := <-captured
	if got.body.TemplateID != "tpl-lock" {
		t.Errorf("template_id = %q, want tpl-lock", got.body.TemplateID)
	}
	if got.body.TemplateData["lock_code"] != "482913" {
		t.Errorf("template_data[lock_code] = %q, want 482913", got.body.TemplateData["lock_code"])
	}
	if got.body.TemplateData["start_time"] != "01/08/2026 14:00" {
		t.Errorf("template_data[start_time] = %q, want 01/08/2026 14:00", got.body.TemplateData["start_time"])
	}
}

func TestNormalizeVNPhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0901234567", "84901234567"},
		{"+84901234567", "84901234567"},
		{"84901234567", "84901234567"},
		{"090 123 4567", "84901234567"},
		{"090-123-4567", "84901234567"},
	}

	for _, c := range cases {
		if got := normalizeVNPhone(c.in); got != c.want {
			t.Errorf("normalizeVNPhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 5: Write `util/notify/composite_test.go`**

```go
package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/johnquangdev/laverte-home/model"
)

type recordingNotifier struct {
	confirmed  int
	lockCode   int
	adminAlert int
	err        error
}

func (r *recordingNotifier) BookingConfirmed(context.Context, *model.Booking) error {
	r.confirmed++
	return r.err
}

func (r *recordingNotifier) LockCode(context.Context, *model.Booking, string) error {
	r.lockCode++
	return r.err
}

func (r *recordingNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error {
	r.adminAlert++
	return r.err
}

func TestCompositeSendsCustomerMessagesOnlyToCustomerChannel(t *testing.T) {
	customer, admin := &recordingNotifier{}, &recordingNotifier{}
	n := NewComposite(customer, admin)
	ctx := context.Background()
	b := &model.Booking{ID: 1}

	if err := n.BookingConfirmed(ctx, b); err != nil {
		t.Fatalf("BookingConfirmed() error = %v", err)
	}
	if err := n.LockCode(ctx, b, "1234"); err != nil {
		t.Fatalf("LockCode() error = %v", err)
	}

	if customer.confirmed != 1 || customer.lockCode != 1 {
		t.Errorf("customer channel = %+v, want confirmed=1 lockCode=1", customer)
	}
	if admin.confirmed+admin.lockCode+admin.adminAlert != 0 {
		t.Errorf("admin channel = %+v, want no calls", admin)
	}
}

func TestCompositeSendsAdminAlertOnlyToAdminChannel(t *testing.T) {
	customer, admin := &recordingNotifier{}, &recordingNotifier{}
	n := NewComposite(customer, admin)

	if err := n.AdminLockCodeMissing(context.Background(), &model.Booking{ID: 1}); err != nil {
		t.Fatalf("AdminLockCodeMissing() error = %v", err)
	}

	if admin.adminAlert != 1 {
		t.Errorf("admin channel adminAlert = %d, want 1", admin.adminAlert)
	}
	if customer.confirmed+customer.lockCode+customer.adminAlert != 0 {
		t.Errorf("customer channel = %+v, want no calls", customer)
	}
}

func TestCompositePropagatesDelegateError(t *testing.T) {
	wantErr := errors.New("zns unreachable")
	n := NewComposite(&recordingNotifier{err: wantErr}, &recordingNotifier{})

	if err := n.BookingConfirmed(context.Background(), &model.Booking{ID: 1}); !errors.Is(err, wantErr) {
		t.Errorf("BookingConfirmed() error = %v, want %v", err, wantErr)
	}
}
```

- [ ] **Step 6: Run the notify tests**

Run: `go test ./util/notify/... -race -v`
Expected: PASS — 7 tests (`TestZNSBookingConfirmedPostsTemplateAndNormalizedPhone`, `TestZNSErrorFieldFailsDespiteHTTP200`, `TestZNSLockCodeSendsTheCode`, `TestNormalizeVNPhone`, `TestCompositeSendsCustomerMessagesOnlyToCustomerChannel`, `TestCompositeSendsAdminAlertOnlyToAdminChannel`, `TestCompositePropagatesDelegateError`), plus the noop tests already in the package from Task 13. No SMTP connection and no call to Zalo are made.

- [ ] **Step 7: Modify `cmd/main.go` — compose the real notifier when both channels are configured**

Replace the single line that Task 13 wrote in the dependency-wiring section:

```go
	notifier := notify.NewNoop()
```

with this block, and add `"net/http"` and `"time"` to the import block if they are not already there:

```go
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
```

`notifier` then replaces `notify.NewNoop()` in the same billing-usecase call Task 14's Step 5 already touched — every other argument unchanged:

```go
	billingUC := billinguc.New(bookings, payments, sepay, notifier, calendarSvc, homes, log, *cfg)
```

- [ ] **Step 8: Build, vet, commit**

```bash
go build ./...
go vet ./...
git add util/notify/zns.go util/notify/smtp_admin.go util/notify/composite.go util/notify/zns_test.go util/notify/composite_test.go cmd/main.go
git commit -m "feat: Zalo ZNS customer notifier + SMTP admin alert, composed behind INotifier"
```

---

---

## Phase 4 — Admin operations, scheduled jobs, and final verification

### Task 16: Admin booking operations (list, walk-in, cancel, complete, no-show, lock code)

**Files:**
- Create: `payload/admin_booking.go`
- Create: `presenter/admin_booking.go`
- Create: `usecase/bookingadmin/interface.go`
- Create: `usecase/bookingadmin/usecase.go`
- Create: `usecase/bookingadmin/lock_code.go`
- Create: `usecase/bookingadmin/usecase_test.go`
- Create: `usecase/bookingadmin/lock_code_test.go`
- Create: `delivery/http/admin/booking_handler.go`
- Create: `delivery/http/admin/booking_route.go`
- Modify: `delivery/http/http.go` — add the `bookingAdminUC` parameter and mount `InitBookings`
- Modify: `delivery/http/http_test.go` — add a stub for the new usecase
- Modify: `cmd/main.go` — construct and pass `bookingAdminUC`

**Interfaces:**
- Consumes: `bookingrepo.IRepository` + `bookingrepo.ErrSlotConflict` (Task 9); `homerepo.IRepository` (Task 7); `blockedslotrepo.IRepository` (Task 10); `paymentrepo.IRepository` (Task 13); `pricinguc.IUseCase` (Task 8); `notify.INotifier`, `gcalendar.ICalendar` (Tasks 14/15); `middleware.ClaimsFromContext` (Task 5); `HandleErrFunc`/`HandleOKFunc` already declared in package `delivery/http/admin` (Task 6) — do **not** redeclare them.
- Produces: `payload.CreateWalkInBookingRequest`, `payload.SetLockCodeRequest`; `presenter.AdminBookingResponse`, `presenter.ToAdminBookingResponse`; `bookingadminuc.IUseCase` (`ListByHomeAndDate`, `CreateWalkIn`, `Cancel`, `Complete`, `NoShow`, `SetLockCode`, `SendLockCode`); `adminhttp.InitBookings`.

- [ ] **Step 1: Write `payload/admin_booking.go`**

```go
package payload

import "time"

type CreateWalkInBookingRequest struct {
	HomeID        uint      `json:"home_id" validate:"required"`
	CustomerName  string    `json:"customer_name" validate:"required"`
	CustomerPhone string    `json:"customer_phone" validate:"required"`
	StartTime     time.Time `json:"start_time" validate:"required"`
	EndTime       time.Time `json:"end_time" validate:"required"`
	BookingType   string    `json:"booking_type" validate:"required"`
	PaidCash      bool      `json:"paid_cash"`
}

type SetLockCodeRequest struct {
	Code string `json:"code" validate:"required"`
}
```

- [ ] **Step 2: Write `presenter/admin_booking.go`**

```go
package presenter

import (
	"time"

	"github.com/johnquangdev/laverte-home/model"
)

// AdminBookingResponse carries the same fields as BookingResponse plus the
// operational ones a guest must never see: the door code, the calendar event
// handle, and which admin walked the booking in.
type AdminBookingResponse struct {
	ID                    uint       `json:"id"`
	HomeID                uint       `json:"home_id"`
	CustomerName          string     `json:"customer_name"`
	CustomerPhone         string     `json:"customer_phone"`
	StartTime             time.Time  `json:"start_time"`
	EndTime               time.Time  `json:"end_time"`
	BookingType           string     `json:"booking_type"`
	ComputedPrice         int64      `json:"computed_price"`
	Status                string     `json:"status"`
	ExpiresAt             *time.Time `json:"expires_at"`
	CreatedAt             time.Time  `json:"created_at"`
	DoorLockCode          *string    `json:"door_lock_code"`
	LockCodeSentAt        *time.Time `json:"lock_code_sent_at"`
	GoogleCalendarEventID string     `json:"google_calendar_event_id"`
	CreatedByAdminID      *uint      `json:"created_by_admin_id"`
}

func ToAdminBookingResponse(b *model.Booking) AdminBookingResponse {
	return AdminBookingResponse{
		ID: b.ID, HomeID: b.HomeID,
		CustomerName: b.CustomerName, CustomerPhone: b.CustomerPhone,
		StartTime: b.StartTime, EndTime: b.EndTime,
		BookingType: b.BookingType, ComputedPrice: b.ComputedPrice,
		Status: b.Status, ExpiresAt: b.ExpiresAt, CreatedAt: b.CreatedAt,
		DoorLockCode: b.DoorLockCode, LockCodeSentAt: b.LockCodeSentAt,
		GoogleCalendarEventID: b.GoogleCalendarEventID,
		CreatedByAdminID:      b.CreatedByAdminID,
	}
}
```

- [ ] **Step 3: Write `usecase/bookingadmin/interface.go`**

```go
package bookingadmin

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]presenter.AdminBookingResponse, error)
	CreateWalkIn(ctx context.Context, req payload.CreateWalkInBookingRequest, adminID uint) (*presenter.AdminBookingResponse, error)
	Cancel(ctx context.Context, id uint) error
	Complete(ctx context.Context, id uint) error
	NoShow(ctx context.Context, id uint) error
	SetLockCode(ctx context.Context, id uint, code string) error
	SendLockCode(ctx context.Context, id uint) error
}
```

- [ ] **Step 4: Write `usecase/bookingadmin/usecase.go`** — struct, constructor, list, walk-in creation

```go
package bookingadmin

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	"github.com/johnquangdev/laverte-home/presenter"
	blockedslotrepo "github.com/johnquangdev/laverte-home/repository/blockedslot"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
	homerepo "github.com/johnquangdev/laverte-home/repository/home"
	paymentrepo "github.com/johnquangdev/laverte-home/repository/payment"
	pricinguc "github.com/johnquangdev/laverte-home/usecase/pricing"
	"github.com/johnquangdev/laverte-home/util/gcalendar"
	"github.com/johnquangdev/laverte-home/util/notify"
)

type UseCase struct {
	bookingRepo     bookingrepo.IRepository
	homeRepo        homerepo.IRepository
	blockedSlotRepo blockedslotrepo.IRepository
	paymentRepo     paymentrepo.IRepository
	pricingUC       pricinguc.IUseCase
	notifier        notify.INotifier
	calendar        gcalendar.ICalendar
	log             *zap.Logger
}

func New(
	bookingRepo bookingrepo.IRepository,
	homeRepo homerepo.IRepository,
	blockedSlotRepo blockedslotrepo.IRepository,
	paymentRepo paymentrepo.IRepository,
	pricingUC pricinguc.IUseCase,
	notifier notify.INotifier,
	calendar gcalendar.ICalendar,
	log *zap.Logger,
) IUseCase {
	return &UseCase{
		bookingRepo:     bookingRepo,
		homeRepo:        homeRepo,
		blockedSlotRepo: blockedSlotRepo,
		paymentRepo:     paymentRepo,
		pricingUC:       pricingUC,
		notifier:        notifier,
		calendar:        calendar,
		log:             log,
	}
}

func (uc *UseCase) ListByHomeAndDate(ctx context.Context, homeID uint, day time.Time) ([]presenter.AdminBookingResponse, error) {
	rows, err := uc.bookingRepo.ListByHomeAndDate(ctx, homeID, day)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]presenter.AdminBookingResponse, 0, len(rows))
	for _, b := range rows {
		out = append(out, presenter.ToAdminBookingResponse(b))
	}
	return out, nil
}

func (uc *UseCase) CreateWalkIn(ctx context.Context, req payload.CreateWalkInBookingRequest, adminID uint) (*presenter.AdminBookingResponse, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.Validation("end_time phai sau start_time")
	}
	if !model.IsValidBookingType(req.BookingType) {
		return nil, apperr.Validation("booking_type phai la 'hourly', 'overnight' hoac 'day'")
	}

	home, err := uc.homeRepo.GetByID(ctx, req.HomeID)
	if err != nil {
		return nil, apperr.NotFound(err)
	}

	blocked, err := uc.blockedSlotRepo.HasOverlap(ctx, req.HomeID, req.StartTime, req.EndTime)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if blocked {
		return nil, apperr.SlotConflict(nil)
	}

	price, err := uc.pricingUC.Compute(ctx, home.Category, req.BookingType, req.StartTime, req.EndTime, time.Now())
	if err != nil {
		return nil, err
	}

	b := &model.Booking{
		HomeID:           req.HomeID,
		CustomerName:     req.CustomerName,
		CustomerPhone:    req.CustomerPhone,
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		BookingType:      req.BookingType,
		ComputedPrice:    price,
		Status:           model.BookingStatusConfirmed,
		CreatedByAdminID: &adminID,
		// A walk-in guest is physically in the room, so the slot is held
		// outright — leaving ExpiresAt nil keeps the pending-expiry sweep from
		// ever reclaiming it.
		ExpiresAt: nil,
	}
	if err := uc.bookingRepo.Create(ctx, b); err != nil {
		if errors.Is(err, bookingrepo.ErrSlotConflict) {
			return nil, apperr.SlotConflict(err)
		}
		return nil, apperr.Internal(err)
	}

	if req.PaidCash {
		now := time.Now()
		p := &model.Payment{
			BookingID: b.ID,
			Provider:  model.PaymentProviderCash,
			Status:    model.PaymentStatusPaid,
			Amount:    price,
			PaidAt:    &now,
		}
		if err := uc.paymentRepo.Create(ctx, p); err != nil {
			return nil, apperr.Internal(err)
		}
		b.PaymentID = &p.ID
		if err := uc.bookingRepo.Update(ctx, b); err != nil {
			return nil, apperr.Internal(err)
		}
	}

	uc.pushCalendarEvent(ctx, home, b)

	resp := presenter.ToAdminBookingResponse(b)
	return &resp, nil
}

// pushCalendarEvent is best-effort, the same stance the SePay webhook takes:
// the booking row is already committed and the guest is already checked in, so
// a Calendar outage must not turn a real stay into a failed request.
func (uc *UseCase) pushCalendarEvent(ctx context.Context, home *model.Home, b *model.Booking) {
	if home.GoogleCalendarID == "" {
		return
	}
	eventID, err := uc.calendar.CreateEvent(ctx, home.GoogleCalendarID, b)
	if err != nil {
		uc.log.Error("walk-in calendar push failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	b.GoogleCalendarEventID = eventID
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		uc.log.Error("persist calendar event id failed", zap.Uint("booking_id", b.ID), zap.Error(err))
	}
}
```

- [ ] **Step 5: Append the status transitions to `usecase/bookingadmin/usecase.go`**

```go
func (uc *UseCase) Cancel(ctx context.Context, id uint) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	if b.Status == model.BookingStatusCancelled || b.Status == model.BookingStatusExpired {
		return apperr.Validation("booking da huy hoac da het han")
	}

	b.Status = model.BookingStatusCancelled
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		return apperr.Internal(err)
	}

	uc.deleteCalendarEvent(ctx, b)
	return nil
}

func (uc *UseCase) Complete(ctx context.Context, id uint) error {
	return uc.transitionFromConfirmed(ctx, id, model.BookingStatusCompleted)
}

func (uc *UseCase) NoShow(ctx context.Context, id uint) error {
	return uc.transitionFromConfirmed(ctx, id, model.BookingStatusNoShow)
}

// transitionFromConfirmed guards the two closing states: a booking that was
// never confirmed has no stay to close out, so the request is a mistake rather
// than a no-op.
func (uc *UseCase) transitionFromConfirmed(ctx context.Context, id uint, status string) error {
	b, err := uc.bookingRepo.GetByID(ctx, id)
	if err != nil {
		return apperr.NotFound(err)
	}
	if b.Status != model.BookingStatusConfirmed {
		return apperr.Validation("chi booking dang 'confirmed' moi doi duoc trang thai nay")
	}
	b.Status = status
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// deleteCalendarEvent mirrors pushCalendarEvent's best-effort stance — the
// cancellation is already persisted, a leftover Calendar event is cleaned up by
// hand.
func (uc *UseCase) deleteCalendarEvent(ctx context.Context, b *model.Booking) {
	if b.GoogleCalendarEventID == "" {
		return
	}
	home, err := uc.homeRepo.GetByID(ctx, b.HomeID)
	if err != nil {
		uc.log.Error("load home for calendar delete failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		return
	}
	if home.GoogleCalendarID == "" {
		return
	}
	if err := uc.calendar.DeleteEvent(ctx, home.GoogleCalendarID, b.GoogleCalendarEventID); err != nil {
		uc.log.Error("calendar event delete failed", zap.Uint("booking_id", b.ID), zap.Error(err))
	}
}
```

- [ ] **Step 6: Write `usecase/bookingadmin/lock_code.go`**

```go
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

	now := time.Now()
	b.LockCodeSentAt = &now
	if err := uc.bookingRepo.Update(ctx, b); err != nil {
		return apperr.Internal(err)
	}
	return nil
}
```

- [ ] **Step 7: Write `usecase/bookingadmin/usecase_test.go`** — hand-written fakes shared by both test files in this package, plus the walk-in and status-transition cases

```go
package bookingadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
	"github.com/johnquangdev/laverte-home/payload"
	bookingrepo "github.com/johnquangdev/laverte-home/repository/booking"
)

type fakeBookingRepo struct {
	rows       map[uint]*model.Booking
	nextID     uint
	createErr  error
	updateErrs map[uint]error
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{rows: map[uint]*model.Booking{}, updateErrs: map[uint]error{}}
}

func (f *fakeBookingRepo) seed(b *model.Booking) *model.Booking {
	f.nextID++
	b.ID = f.nextID
	f.rows[b.ID] = b
	return b
}

func (f *fakeBookingRepo) Create(_ context.Context, b *model.Booking) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.seed(b)
	return nil
}

func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	b, ok := f.rows[id]
	if !ok {
		return nil, errors.New("booking not found")
	}
	return b, nil
}

func (f *fakeBookingRepo) Update(_ context.Context, b *model.Booking) error {
	if err := f.updateErrs[b.ID]; err != nil {
		return err
	}
	f.rows[b.ID] = b
	return nil
}

func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) ListExpiredPending(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListByHomeAndDate(_ context.Context, homeID uint, _ time.Time) ([]*model.Booking, error) {
	var out []*model.Booking
	for _, b := range f.rows {
		if b.HomeID == homeID {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeBookingRepo) ListUpcomingMissingLockCode(context.Context, time.Time, time.Duration) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListReadyToSendLockCode(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

// CountConfirmedBetween joins bookingrepo.IRepository in Task 18; the fake
// carries it from the start so this file needs no edit then.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type fakeHomeRepo struct{ homes map[uint]*model.Home }

func (f *fakeHomeRepo) Create(_ context.Context, h *model.Home) error { f.homes[h.ID] = h; return nil }
func (f *fakeHomeRepo) Update(_ context.Context, h *model.Home) error { f.homes[h.ID] = h; return nil }
func (f *fakeHomeRepo) GetByID(_ context.Context, id uint) (*model.Home, error) {
	h, ok := f.homes[id]
	if !ok {
		return nil, errors.New("home not found")
	}
	return h, nil
}
func (f *fakeHomeRepo) List(context.Context) ([]*model.Home, error) { return nil, nil }

type fakeBlockedSlotRepo struct{ overlap bool }

func (f *fakeBlockedSlotRepo) Create(context.Context, *model.BlockedSlot) error { return nil }
func (f *fakeBlockedSlotRepo) Delete(context.Context, uint) error               { return nil }
func (f *fakeBlockedSlotRepo) ListByHome(context.Context, uint) ([]*model.BlockedSlot, error) {
	return nil, nil
}
func (f *fakeBlockedSlotRepo) HasOverlap(context.Context, uint, time.Time, time.Time) (bool, error) {
	return f.overlap, nil
}

type fakePaymentRepo struct {
	rows   map[uint]*model.Payment
	nextID uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{rows: map[uint]*model.Payment{}}
}

func (f *fakePaymentRepo) Create(_ context.Context, p *model.Payment) error {
	f.nextID++
	p.ID = f.nextID
	f.rows[p.ID] = p
	return nil
}
func (f *fakePaymentRepo) GetByID(_ context.Context, id uint) (*model.Payment, error) {
	p, ok := f.rows[id]
	if !ok {
		return nil, errors.New("payment not found")
	}
	return p, nil
}
func (f *fakePaymentRepo) GetByBookingID(_ context.Context, bookingID uint) (*model.Payment, error) {
	for _, p := range f.rows {
		if p.BookingID == bookingID {
			return p, nil
		}
	}
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return false, nil
}
func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

// MarkExpiredIfPending joins paymentrepo.IRepository in Task 17; the fake
// carries it from the start so this file needs no edit then.
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }

type fakePricing struct {
	price int64
	err   error
}

func (f *fakePricing) Compute(context.Context, string, string, time.Time, time.Time, time.Time) (int64, error) {
	return f.price, f.err
}

type fakeNotifier struct {
	lockCodeCalls int
	lockCodeErr   error
	alertCalls    int
}

func (f *fakeNotifier) BookingConfirmed(context.Context, *model.Booking) error { return nil }
func (f *fakeNotifier) LockCode(context.Context, *model.Booking, string) error {
	f.lockCodeCalls++
	return f.lockCodeErr
}
func (f *fakeNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error {
	f.alertCalls++
	return nil
}

type fakeCalendar struct {
	eventID      string
	createErr    error
	createCalls  int
	deleteCalls  int
	deletedEvent string
}

func (f *fakeCalendar) CreateEvent(context.Context, string, *model.Booking) (string, error) {
	f.createCalls++
	return f.eventID, f.createErr
}
func (f *fakeCalendar) DeleteEvent(_ context.Context, _ string, eventID string) error {
	f.deleteCalls++
	f.deletedEvent = eventID
	return nil
}

type deps struct {
	bookings *fakeBookingRepo
	homes    *fakeHomeRepo
	blocked  *fakeBlockedSlotRepo
	payments *fakePaymentRepo
	pricing  *fakePricing
	notifier *fakeNotifier
	calendar *fakeCalendar
}

func newTestUseCase() (*UseCase, *deps) {
	d := &deps{
		bookings: newFakeBookingRepo(),
		homes: &fakeHomeRepo{homes: map[uint]*model.Home{
			1: {ID: 1, Name: "Nest 1", Category: model.HomeCategoryNest, GoogleCalendarID: "cal-1", IsActive: true},
		}},
		blocked:  &fakeBlockedSlotRepo{},
		payments: newFakePaymentRepo(),
		pricing:  &fakePricing{price: 300000},
		notifier: &fakeNotifier{},
		calendar: &fakeCalendar{eventID: "evt-1"},
	}
	uc := New(d.bookings, d.homes, d.blocked, d.payments, d.pricing, d.notifier, d.calendar, zap.NewNop()).(*UseCase)
	return uc, d
}

func walkInRequest() payload.CreateWalkInBookingRequest {
	start := time.Now().Add(time.Hour).Truncate(time.Second)
	return payload.CreateWalkInBookingRequest{
		HomeID: 1, CustomerName: "Khach", CustomerPhone: "0900000001",
		StartTime: start, EndTime: start.Add(2 * time.Hour),
		BookingType: model.BookingTypeHourly,
	}
}

func TestCreateWalkInIsConfirmedAndAttributedToAdmin(t *testing.T) {
	uc, d := newTestUseCase()

	resp, err := uc.CreateWalkIn(context.Background(), walkInRequest(), 77)
	if err != nil {
		t.Fatalf("CreateWalkIn() error = %v", err)
	}
	if resp.Status != model.BookingStatusConfirmed {
		t.Errorf("Status = %q, want %q", resp.Status, model.BookingStatusConfirmed)
	}
	if resp.CreatedByAdminID == nil || *resp.CreatedByAdminID != 77 {
		t.Errorf("CreatedByAdminID = %v, want 77", resp.CreatedByAdminID)
	}
	if resp.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil (a walk-in never expires)", resp.ExpiresAt)
	}
	if d.calendar.createCalls != 1 {
		t.Errorf("calendar.CreateEvent calls = %d, want 1", d.calendar.createCalls)
	}
	stored := d.bookings.rows[resp.ID]
	if stored.GoogleCalendarEventID != "evt-1" {
		t.Errorf("GoogleCalendarEventID = %q, want evt-1", stored.GoogleCalendarEventID)
	}
}

func TestCreateWalkInWithPaidCashCreatesCashPayment(t *testing.T) {
	uc, d := newTestUseCase()
	req := walkInRequest()
	req.PaidCash = true

	resp, err := uc.CreateWalkIn(context.Background(), req, 77)
	if err != nil {
		t.Fatalf("CreateWalkIn() error = %v", err)
	}

	p, err := d.payments.GetByBookingID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("GetByBookingID() error = %v", err)
	}
	if p.Provider != model.PaymentProviderCash || p.Status != model.PaymentStatusPaid {
		t.Errorf("payment = %q/%q, want %q/%q", p.Provider, p.Status, model.PaymentProviderCash, model.PaymentStatusPaid)
	}
	if p.Amount != 300000 {
		t.Errorf("Amount = %d, want 300000", p.Amount)
	}
	if p.PaidAt == nil {
		t.Error("PaidAt = nil, want a timestamp")
	}
	if stored := d.bookings.rows[resp.ID]; stored.PaymentID == nil || *stored.PaymentID != p.ID {
		t.Errorf("booking.PaymentID = %v, want %d", stored.PaymentID, p.ID)
	}
}

func TestCreateWalkInRejectsBlockedSlot(t *testing.T) {
	uc, d := newTestUseCase()
	d.blocked.overlap = true

	_, err := uc.CreateWalkIn(context.Background(), walkInRequest(), 77)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeSlotConflict {
		t.Fatalf("CreateWalkIn() error = %v, want CodeSlotConflict", err)
	}
}

func TestCreateWalkInMapsRepoSlotConflict(t *testing.T) {
	uc, d := newTestUseCase()
	d.bookings.createErr = bookingrepo.ErrSlotConflict

	_, err := uc.CreateWalkIn(context.Background(), walkInRequest(), 77)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeSlotConflict {
		t.Fatalf("CreateWalkIn() error = %v, want CodeSlotConflict", err)
	}
}

func TestCancelRejectsAlreadyCancelled(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusCancelled})

	err := uc.Cancel(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Cancel() error = %v, want CodeValidation", err)
	}
}

func TestCancelDeletesCalendarEvent(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, GoogleCalendarEventID: "evt-9"})

	if err := uc.Cancel(context.Background(), b.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if b.Status != model.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q", b.Status, model.BookingStatusCancelled)
	}
	if d.calendar.deleteCalls != 1 || d.calendar.deletedEvent != "evt-9" {
		t.Errorf("DeleteEvent calls = %d, event = %q; want 1, evt-9", d.calendar.deleteCalls, d.calendar.deletedEvent)
	}
}

func TestCompleteRejectsPendingPayment(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment})

	err := uc.Complete(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Complete() error = %v, want CodeValidation", err)
	}
}
```

- [ ] **Step 8: Write `usecase/bookingadmin/lock_code_test.go`** (reuses the fakes from `usecase_test.go` — same package)

```go
package bookingadmin

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
)

func TestSetLockCodeRejectsNonConfirmedBooking(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment})

	err := uc.SetLockCode(context.Background(), b.ID, "1234")
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("SetLockCode() error = %v, want CodeValidation", err)
	}
	if b.DoorLockCode != nil {
		t.Errorf("DoorLockCode = %v, want nil", b.DoorLockCode)
	}
}

func TestSetLockCodeStoresCode(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed})

	if err := uc.SetLockCode(context.Background(), b.ID, "4321"); err != nil {
		t.Fatalf("SetLockCode() error = %v", err)
	}
	if b.DoorLockCode == nil || *b.DoorLockCode != "4321" {
		t.Errorf("DoorLockCode = %v, want 4321", b.DoorLockCode)
	}
}

func TestSendLockCodeRejectsMissingCode(t *testing.T) {
	uc, d := newTestUseCase()
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed})

	err := uc.SendLockCode(context.Background(), b.ID)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("SendLockCode() error = %v, want CodeValidation", err)
	}
	if d.notifier.lockCodeCalls != 0 {
		t.Errorf("notifier.LockCode calls = %d, want 0", d.notifier.lockCodeCalls)
	}
}

func TestSendLockCodeSendsOnlyOnce(t *testing.T) {
	uc, d := newTestUseCase()
	code := "1357"
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, DoorLockCode: &code})

	if err := uc.SendLockCode(context.Background(), b.ID); err != nil {
		t.Fatalf("first SendLockCode() error = %v", err)
	}
	if b.LockCodeSentAt == nil {
		t.Fatal("LockCodeSentAt = nil after send, want a timestamp")
	}
	if err := uc.SendLockCode(context.Background(), b.ID); err != nil {
		t.Fatalf("second SendLockCode() error = %v", err)
	}
	if d.notifier.lockCodeCalls != 1 {
		t.Errorf("notifier.LockCode calls = %d, want 1", d.notifier.lockCodeCalls)
	}
}

func TestSendLockCodePropagatesNotifierError(t *testing.T) {
	uc, d := newTestUseCase()
	code := "2468"
	b := d.bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusConfirmed, DoorLockCode: &code})
	d.notifier.lockCodeErr = errors.New("zns down")

	if err := uc.SendLockCode(context.Background(), b.ID); err == nil {
		t.Fatal("SendLockCode() = nil error, want the notifier failure surfaced to the admin")
	}
	if b.LockCodeSentAt != nil {
		t.Errorf("LockCodeSentAt = %v, want nil so a retry can still deliver", b.LockCodeSentAt)
	}
}
```

- [ ] **Step 9: Run the usecase tests**

Run: `go test ./usecase/bookingadmin/... -v`
Expected: PASS — all 11 tests across `usecase_test.go` and `lock_code_test.go`.

- [ ] **Step 10: Write `delivery/http/admin/booking_handler.go`**

```go
package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/johnquangdev/laverte-home/delivery/http/middleware"
	"github.com/johnquangdev/laverte-home/payload"
	bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"
)

// dateLayout is the only date format admin query params accept.
const dateLayout = "2006-01-02"

type BookingHandler struct {
	uc        bookingadminuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newBookingHandler(uc bookingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *BookingHandler {
	return &BookingHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *BookingHandler) list(c echo.Context) error {
	homeID, err := strconv.ParseUint(c.QueryParam("home_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid home_id"})
	}

	day := time.Now()
	if raw := c.QueryParam("date"); raw != "" {
		day, err = time.ParseInLocation(dateLayout, raw, time.Now().Location())
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid date, want YYYY-MM-DD"})
		}
	}

	resp, err := h.uc.ListByHomeAndDate(c.Request().Context(), uint(homeID), day)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BookingHandler) create(c echo.Context) error {
	var req payload.CreateWalkInBookingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	adminID := middleware.ClaimsFromContext(c).UserID
	resp, err := h.uc.CreateWalkIn(c.Request().Context(), req, adminID)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}

func (h *BookingHandler) setLockCode(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req payload.SetLockCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if err := h.uc.SetLockCode(c.Request().Context(), uint(id), req.Code); err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, map[string]bool{"ok": true})
}

// idAction adapts the four id-only operations (cancel/complete/no-show/
// send-lock-code) that all answer `{"ok":true}`.
func (h *BookingHandler) idAction(action func(ctx context.Context, id uint) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
		}
		if err := action(c.Request().Context(), uint(id)); err != nil {
			return h.handleErr(c, err)
		}
		return h.handleOK(c, map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 11: Write `delivery/http/admin/booking_route.go`**

```go
package admin

import (
	"github.com/labstack/echo/v4"

	bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"
)

func InitBookings(g *echo.Group, uc bookingadminuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newBookingHandler(uc, handleErr, handleOK)
	bookings := g.Group("/bookings")
	bookings.GET("", h.list)
	bookings.POST("", h.create)
	bookings.PATCH("/:id/cancel", h.idAction(uc.Cancel))
	bookings.PATCH("/:id/complete", h.idAction(uc.Complete))
	bookings.PATCH("/:id/no-show", h.idAction(uc.NoShow))
	bookings.PATCH("/:id/lock-code", h.setLockCode)
	bookings.POST("/:id/send-lock-code", h.idAction(uc.SendLockCode))
}
```

- [ ] **Step 12: Modify `delivery/http/http.go`** — add the parameter and mount the routes

Add to the import block:

```go
	bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"
```

Add one field to the `Deps` struct (Task 6):

```go
	BookingAdminUC bookingadminuc.IUseCase
```

and mount it on the shared `adminGroup`, after the existing `adminhttp.InitBlockedSlots(...)` line:

```go
	adminhttp.InitBookings(adminGroup, deps.BookingAdminUC, handleErr, handleOK)
```

- [ ] **Step 13: Modify `delivery/http/http_test.go`** — add a stub for the new usecase and pass it to `newTestServer()`

```go
type stubBookingAdminUC struct{}

func (stubBookingAdminUC) ListByHomeAndDate(context.Context, uint, time.Time) ([]presenter.AdminBookingResponse, error) {
	return nil, nil
}
func (stubBookingAdminUC) CreateWalkIn(context.Context, payload.CreateWalkInBookingRequest, uint) (*presenter.AdminBookingResponse, error) {
	return nil, nil
}
func (stubBookingAdminUC) Cancel(context.Context, uint) error          { return nil }
func (stubBookingAdminUC) Complete(context.Context, uint) error        { return nil }
func (stubBookingAdminUC) NoShow(context.Context, uint) error          { return nil }
func (stubBookingAdminUC) SetLockCode(context.Context, uint, string) error { return nil }
func (stubBookingAdminUC) SendLockCode(context.Context, uint) error    { return nil }
```

Add `BookingAdminUC: stubBookingAdminUC{},` to the `Deps` literal inside `newTestServer()`, and add `"time"` to the imports if it isn't there yet.

- [ ] **Step 14: Modify `cmd/main.go`** — construct the usecase and pass it in

After the existing repo/usecase construction (`bookings`, `homes`, `payments`, `blockedSlots`, `pricingUC`, `notifier`, `calendarSvc` already exist from Tasks 9–15), add:

```go
	bookingAdminUC := bookingadminuc.New(bookings, homes, blockedSlots, payments, pricingUC, notifier, calendarSvc, log)
```

Add the import `bookingadminuc "github.com/johnquangdev/laverte-home/usecase/bookingadmin"` and add `BookingAdminUC: bookingAdminUC,` to the `httpserver.Deps{...}` literal.

- [ ] **Step 15: Build, vet, run the full suite, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add payload/admin_booking.go presenter/admin_booking.go usecase/bookingadmin delivery/http/admin/booking_handler.go delivery/http/admin/booking_route.go delivery/http/http.go delivery/http/http_test.go cmd/main.go
git commit -m "feat: admin booking ops — walk-in, cancel/complete/no-show, lock code"
```

---

### Task 17: Cron jobs — expire pending bookings, lock-code reminder + auto-send

**Files:**
- Modify: `repository/payment/interface.go` — add `MarkExpiredIfPending`
- Modify: `repository/payment/pg.go` — implement `MarkExpiredIfPending`
- Create: `usecase/bookingjobs/interface.go`
- Create: `usecase/bookingjobs/usecase.go`
- Create: `usecase/bookingjobs/usecase_test.go`
- Create: `delivery/job/job.go`
- Create: `delivery/job/booking_expiry.go`
- Create: `delivery/job/lock_code.go`
- Modify: `cmd/main.go` — start/stop the cron registry

**Interfaces:**
- Consumes: `bookingrepo.IRepository` (`ListExpiredPending`, `ListUpcomingMissingLockCode`, `ListReadyToSendLockCode`, `Update`) (Task 9); `paymentrepo.IRepository` (Task 13); `notify.INotifier` (Task 14); `cfg.BookingCheckinAlertLeadMinutes` (Task 1).
- Produces: `paymentrepo.MarkExpiredIfPending`; `bookingjobsuc.IUseCase` (`ExpirePendingBookings`, `AlertMissingLockCodes`, `SendDueLockCodes`); `job.New(uc, log) *Job`, `(*Job).Start()`, `(*Job).Stop()`.

- [ ] **Step 1: Modify `repository/payment/interface.go`** — append one method to `IRepository`

```go
	// MarkExpiredIfPending flips a payment to expired only while it is still
	// pending, so the expiry sweep can never overwrite a webhook that already
	// marked it paid. The payment repo deliberately exposes no generic Update —
	// every status change is one of these guarded transitions.
	MarkExpiredIfPending(ctx context.Context, paymentID uint) error
```

- [ ] **Step 2: Modify `repository/payment/pg.go`** — append the implementation

```go
func (r *pgRepository) MarkExpiredIfPending(ctx context.Context, paymentID uint) error {
	return r.getDB(ctx).Model(&model.Payment{}).
		Where("id = ? AND status = ?", paymentID, model.PaymentStatusPending).
		Update("status", model.PaymentStatusExpired).Error
}
```

Any hand-written `paymentrepo.IRepository` fake in an earlier task's test file needs this method too. The fake in `usecase/bookingadmin/usecase_test.go` (Task 16) already carries it; add it to the payment/booking usecase fakes from Tasks 12–13 if they exist:

```go
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }
```

- [ ] **Step 3: Write `usecase/bookingjobs/interface.go`**

```go
package bookingjobs

import "context"

// IUseCase holds the three periodic sweeps driven by delivery/job. Each one is
// a whole-batch operation: it returns an error only when the batch could not be
// read at all.
type IUseCase interface {
	ExpirePendingBookings(ctx context.Context) error
	AlertMissingLockCodes(ctx context.Context) error
	SendDueLockCodes(ctx context.Context) error
}
```

- [ ] **Step 4: Write `usecase/bookingjobs/usecase.go`**

```go
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
		b.Status = model.BookingStatusExpired
		if err := uc.bookingRepo.Update(ctx, b); err != nil {
			uc.log.Error("expire booking failed", zap.Uint("booking_id", b.ID), zap.Error(err))
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
		// every tick, so it is written only after the alert actually went out.
		now := time.Now()
		b.LockCodeAlertSentAt = &now
		if err := uc.bookingRepo.Update(ctx, b); err != nil {
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
		if err := uc.notifier.LockCode(ctx, b, *b.DoorLockCode); err != nil {
			uc.log.Error("lock-code send failed", zap.Uint("booking_id", b.ID), zap.Error(err))
			continue
		}
		now := time.Now()
		b.LockCodeSentAt = &now
		if err := uc.bookingRepo.Update(ctx, b); err != nil {
			uc.log.Error("mark lock-code sent failed", zap.Uint("booking_id", b.ID), zap.Error(err))
		}
	}
	return nil
}
```

- [ ] **Step 5: Write `usecase/bookingjobs/usecase_test.go`**

```go
package bookingjobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	"github.com/johnquangdev/laverte-home/model"
)

type fakeBookingRepo struct {
	rows       map[uint]*model.Booking
	nextID     uint
	updateErrs map[uint]error
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{rows: map[uint]*model.Booking{}, updateErrs: map[uint]error{}}
}

func (f *fakeBookingRepo) seed(b *model.Booking) *model.Booking {
	f.nextID++
	b.ID = f.nextID
	f.rows[b.ID] = b
	return b
}

func (f *fakeBookingRepo) Create(_ context.Context, b *model.Booking) error { f.seed(b); return nil }
func (f *fakeBookingRepo) GetByID(_ context.Context, id uint) (*model.Booking, error) {
	b, ok := f.rows[id]
	if !ok {
		return nil, errors.New("booking not found")
	}
	return b, nil
}
func (f *fakeBookingRepo) Update(_ context.Context, b *model.Booking) error {
	if err := f.updateErrs[b.ID]; err != nil {
		return err
	}
	f.rows[b.ID] = b
	return nil
}
func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) ListExpiredPending(_ context.Context, now time.Time) ([]*model.Booking, error) {
	out := make([]*model.Booking, 0, len(f.rows))
	for i := uint(1); i <= f.nextID; i++ {
		b := f.rows[i]
		if b == nil || b.Status != model.BookingStatusPendingPayment {
			continue
		}
		if b.ExpiresAt != nil && b.ExpiresAt.Before(now) {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeBookingRepo) ListByHomeAndDate(context.Context, uint, time.Time) ([]*model.Booking, error) {
	return nil, nil
}

// ListUpcomingMissingLockCode mirrors the real query's filters — including the
// lock_code_alert_sent_at IS NULL clause the alert sweep relies on.
func (f *fakeBookingRepo) ListUpcomingMissingLockCode(_ context.Context, now time.Time, leadTime time.Duration) ([]*model.Booking, error) {
	out := make([]*model.Booking, 0, len(f.rows))
	for i := uint(1); i <= f.nextID; i++ {
		b := f.rows[i]
		if b == nil || b.Status != model.BookingStatusConfirmed {
			continue
		}
		if b.DoorLockCode != nil || b.LockCodeAlertSentAt != nil {
			continue
		}
		if b.StartTime.After(now) && !b.StartTime.After(now.Add(leadTime)) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBookingRepo) ListReadyToSendLockCode(_ context.Context, now time.Time) ([]*model.Booking, error) {
	out := make([]*model.Booking, 0, len(f.rows))
	for i := uint(1); i <= f.nextID; i++ {
		b := f.rows[i]
		if b == nil || b.Status != model.BookingStatusConfirmed || b.LockCodeSentAt != nil {
			continue
		}
		if !b.StartTime.After(now) {
			out = append(out, b)
		}
	}
	return out, nil
}

// CountConfirmedBetween joins bookingrepo.IRepository in Task 18; the fake
// carries it from the start so this file needs no edit then.
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

type fakePaymentRepo struct {
	rows   map[uint]*model.Payment
	nextID uint
}

func newFakePaymentRepo() *fakePaymentRepo {
	return &fakePaymentRepo{rows: map[uint]*model.Payment{}}
}

func (f *fakePaymentRepo) seed(p *model.Payment) *model.Payment {
	f.nextID++
	p.ID = f.nextID
	f.rows[p.ID] = p
	return p
}

func (f *fakePaymentRepo) Create(_ context.Context, p *model.Payment) error { f.seed(p); return nil }
func (f *fakePaymentRepo) GetByID(_ context.Context, id uint) (*model.Payment, error) {
	p, ok := f.rows[id]
	if !ok {
		return nil, errors.New("payment not found")
	}
	return p, nil
}
func (f *fakePaymentRepo) GetByBookingID(_ context.Context, bookingID uint) (*model.Payment, error) {
	for _, p := range f.rows {
		if p.BookingID == bookingID {
			return p, nil
		}
	}
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return false, nil
}
func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakePaymentRepo) MarkExpiredIfPending(_ context.Context, paymentID uint) error {
	p, ok := f.rows[paymentID]
	if !ok {
		return errors.New("payment not found")
	}
	if p.Status == model.PaymentStatusPending {
		p.Status = model.PaymentStatusExpired
	}
	return nil
}

type fakeNotifier struct {
	lockCodeCalls int
	alertCalls    int
}

func (f *fakeNotifier) BookingConfirmed(context.Context, *model.Booking) error { return nil }
func (f *fakeNotifier) LockCode(context.Context, *model.Booking, string) error {
	f.lockCodeCalls++
	return nil
}
func (f *fakeNotifier) AdminLockCodeMissing(context.Context, *model.Booking) error {
	f.alertCalls++
	return nil
}

func newTestUseCase() (*UseCase, *fakeBookingRepo, *fakePaymentRepo, *fakeNotifier) {
	bookings := newFakeBookingRepo()
	payments := newFakePaymentRepo()
	notifier := &fakeNotifier{}
	cfg := config.Config{BookingCheckinAlertLeadMinutes: 30}
	uc := New(bookings, payments, notifier, zap.NewNop(), cfg).(*UseCase)
	return uc, bookings, payments, notifier
}

func TestExpirePendingBookingsExpiresBookingAndPayment(t *testing.T) {
	uc, bookings, payments, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	b := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	p := payments.seed(&model.Payment{BookingID: b.ID, Provider: model.PaymentProviderSePay, Status: model.PaymentStatusPending, Amount: 300000})
	b.PaymentID = &p.ID

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v", err)
	}
	if b.Status != model.BookingStatusExpired {
		t.Errorf("booking Status = %q, want %q", b.Status, model.BookingStatusExpired)
	}
	if p.Status != model.PaymentStatusExpired {
		t.Errorf("payment Status = %q, want %q", p.Status, model.PaymentStatusExpired)
	}
}

func TestExpirePendingBookingsLeavesPaidPaymentAlone(t *testing.T) {
	uc, bookings, payments, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	paidAt := time.Now()
	b := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	p := payments.seed(&model.Payment{BookingID: b.ID, Provider: model.PaymentProviderSePay, Status: model.PaymentStatusPaid, Amount: 300000, PaidAt: &paidAt})
	b.PaymentID = &p.ID

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v", err)
	}
	if p.Status != model.PaymentStatusPaid {
		t.Errorf("payment Status = %q, want %q", p.Status, model.PaymentStatusPaid)
	}
}

func TestExpirePendingBookingsContinuesAfterRowFailure(t *testing.T) {
	uc, bookings, _, _ := newTestUseCase()
	past := time.Now().Add(-time.Minute)
	broken := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	good := bookings.seed(&model.Booking{HomeID: 1, Status: model.BookingStatusPendingPayment, ExpiresAt: &past})
	bookings.updateErrs[broken.ID] = errors.New("write conflict")

	if err := uc.ExpirePendingBookings(context.Background()); err != nil {
		t.Fatalf("ExpirePendingBookings() error = %v, want nil (per-row failures are logged)", err)
	}
	if good.Status != model.BookingStatusExpired {
		t.Errorf("second booking Status = %q, want %q — a failed row must not abort the sweep", good.Status, model.BookingStatusExpired)
	}
}

func TestAlertMissingLockCodesAlertsOnceOnly(t *testing.T) {
	uc, bookings, _, notifier := newTestUseCase()
	bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(10 * time.Minute),
	})

	if err := uc.AlertMissingLockCodes(context.Background()); err != nil {
		t.Fatalf("first AlertMissingLockCodes() error = %v", err)
	}
	if bookings.rows[1].LockCodeAlertSentAt == nil {
		t.Fatal("LockCodeAlertSentAt = nil after alert, want a timestamp")
	}
	if err := uc.AlertMissingLockCodes(context.Background()); err != nil {
		t.Fatalf("second AlertMissingLockCodes() error = %v", err)
	}
	if notifier.alertCalls != 1 {
		t.Errorf("AdminLockCodeMissing calls = %d, want 1", notifier.alertCalls)
	}
}

func TestSendDueLockCodesSendsAndMarks(t *testing.T) {
	uc, bookings, _, notifier := newTestUseCase()
	code := "1234"
	due := bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(-time.Minute), DoorLockCode: &code,
	})
	noCode := bookings.seed(&model.Booking{
		HomeID: 1, Status: model.BookingStatusConfirmed,
		StartTime: time.Now().Add(-time.Minute),
	})

	if err := uc.SendDueLockCodes(context.Background()); err != nil {
		t.Fatalf("SendDueLockCodes() error = %v", err)
	}
	if notifier.lockCodeCalls != 1 {
		t.Errorf("LockCode calls = %d, want 1", notifier.lockCodeCalls)
	}
	if due.LockCodeSentAt == nil {
		t.Error("LockCodeSentAt = nil for the due booking, want a timestamp")
	}
	if noCode.LockCodeSentAt != nil {
		t.Error("LockCodeSentAt set for a booking with no code, want nil")
	}
}
```

- [ ] **Step 6: Run the usecase tests**

Run: `go test ./usecase/bookingjobs/... -v`
Expected: PASS — 5 tests.

- [ ] **Step 7: Write `delivery/job/job.go`** (thin registry, mirroring lumen's `delivery/job` shape)

```go
package job

import (
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	bookingjobsuc "github.com/johnquangdev/laverte-home/usecase/bookingjobs"
)

type Job struct {
	cron *cron.Cron
	uc   bookingjobsuc.IUseCase
	log  *zap.Logger
}

func New(uc bookingjobsuc.IUseCase, log *zap.Logger) *Job {
	c := cron.New()
	j := &Job{cron: c, uc: uc, log: log}
	// AddFunc only fails on a malformed spec, and these specs are constants.
	_, _ = c.AddFunc("* * * * *", j.expirePendingBookings)
	_, _ = c.AddFunc("*/5 * * * *", j.alertMissingLockCodes)
	_, _ = c.AddFunc("* * * * *", j.sendDueLockCodes)
	return j
}

func (j *Job) Start() { j.cron.Start() }
func (j *Job) Stop()  { j.cron.Stop() }
```

- [ ] **Step 8: Write `delivery/job/booking_expiry.go`**

```go
package job

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// jobTimeout bounds one tick: the expiry and lock-code sweeps run every minute,
// so a hung run must be cut loose before the next one is due.
const jobTimeout = 30 * time.Second

func (j *Job) expirePendingBookings() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	if err := j.uc.ExpirePendingBookings(ctx); err != nil {
		j.log.Error("expire pending bookings job failed", zap.Error(err))
	}
}
```

- [ ] **Step 9: Write `delivery/job/lock_code.go`**

```go
package job

import (
	"context"

	"go.uber.org/zap"
)

func (j *Job) alertMissingLockCodes() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	if err := j.uc.AlertMissingLockCodes(ctx); err != nil {
		j.log.Error("lock-code alert job failed", zap.Error(err))
	}
}

func (j *Job) sendDueLockCodes() {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	if err := j.uc.SendDueLockCodes(ctx); err != nil {
		j.log.Error("lock-code send job failed", zap.Error(err))
	}
}
```

- [ ] **Step 10: Modify `cmd/main.go`** — build the jobs usecase and start the cron registry

Add the imports:

```go
	"github.com/johnquangdev/laverte-home/delivery/job"
	bookingjobsuc "github.com/johnquangdev/laverte-home/usecase/bookingjobs"
```

After the other usecases are constructed and before `srv.Start()`:

```go
	bookingJobsUC := bookingjobsuc.New(bookings, payments, notifier, log, *cfg)

	j := job.New(bookingJobsUC, log)
	j.Start()
	defer j.Stop()
```

- [ ] **Step 11: Build, vet, run the full suite, commit**

```bash
go build ./...
go vet ./...
go test ./...
git add repository/payment usecase/bookingjobs delivery/job cmd/main.go
git commit -m "feat: cron jobs for booking expiry and lock-code reminder/auto-send"
```

---

### Task 18: Admin revenue overview + final verification

**Files:**
- Modify: `repository/booking/interface.go` — add `CountConfirmedBetween`
- Modify: `repository/booking/pg.go` — implement `CountConfirmedBetween`
- Create: `presenter/overview.go`
- Create: `usecase/overview/interface.go`
- Create: `usecase/overview/usecase.go`
- Create: `usecase/overview/usecase_test.go`
- Create: `delivery/http/admin/overview_handler.go`
- Create: `delivery/http/admin/overview_route.go`
- Modify: `delivery/http/http.go`, `delivery/http/http_test.go`, `cmd/main.go` — wire the usecase

**Interfaces:**
- Consumes: `paymentrepo.SumPaidBetween` (Task 13); `bookingrepo.IRepository` (Task 9); `HandleErrFunc`/`HandleOKFunc` and the `dateLayout` const already declared in package `delivery/http/admin` (Tasks 6 and 16) — do **not** redeclare any of them.
- Produces: `bookingrepo.CountConfirmedBetween`; `presenter.OverviewResponse`; `overviewuc.IUseCase` (`Summary`); `adminhttp.InitOverview`.

- [ ] **Step 1: Modify `repository/booking/interface.go`** — append one method to `IRepository` (this file was written in Task 9; the method is appended to the existing interface, nothing is replaced)

```go
	// CountConfirmedBetween counts bookings that actually occupy the property —
	// confirmed plus completed — whose start_time falls in [from, to).
	CountConfirmedBetween(ctx context.Context, from, to time.Time) (int64, error)
```

- [ ] **Step 2: Modify `repository/booking/pg.go`** — append the implementation below the existing methods

```go
func (r *pgRepository) CountConfirmedBetween(ctx context.Context, from, to time.Time) (int64, error) {
	var n int64
	err := r.getDB(ctx).Model(&model.Booking{}).
		Where("status IN ? AND start_time >= ? AND start_time < ?",
			[]string{model.BookingStatusConfirmed, model.BookingStatusCompleted}, from, to).
		Count(&n).Error
	return n, err
}
```

Every hand-written `bookingrepo.IRepository` fake now needs this method. The fakes in `usecase/bookingadmin/usecase_test.go` (Task 16) and `usecase/bookingjobs/usecase_test.go` (Task 17) already carry it; add it to any booking-usecase fake from Tasks 12–13:

```go
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}
```

- [ ] **Step 3: Write `presenter/overview.go`**

```go
package presenter

import "time"

type OverviewResponse struct {
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	TotalRevenueVND int64     `json:"total_revenue_vnd"`
	BookingCount    int64     `json:"booking_count"`
}
```

- [ ] **Step 4: Write `usecase/overview/interface.go`**

```go
package overview

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-home/presenter"
)

type IUseCase interface {
	Summary(ctx context.Context, from, to time.Time) (*presenter.OverviewResponse, error)
}
```

- [ ] **Step 5: Write `usecase/overview/usecase.go`**

```go
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
```

- [ ] **Step 6: Write `usecase/overview/usecase_test.go`**

```go
package overview

import (
	"context"
	"errors"
	"testing"
	"time"

	apperr "github.com/johnquangdev/laverte-home/errors"
	"github.com/johnquangdev/laverte-home/model"
)

type fakePaymentRepo struct {
	sum int64
	err error
}

func (f *fakePaymentRepo) Create(context.Context, *model.Payment) error { return nil }
func (f *fakePaymentRepo) GetByID(context.Context, uint) (*model.Payment, error) {
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) GetByBookingID(context.Context, uint) (*model.Payment, error) {
	return nil, errors.New("payment not found")
}
func (f *fakePaymentRepo) MarkPaidIfPending(context.Context, uint, string, time.Time) (bool, error) {
	return false, nil
}
func (f *fakePaymentRepo) MarkExpiredIfPending(context.Context, uint) error { return nil }
func (f *fakePaymentRepo) SumPaidBetween(context.Context, time.Time, time.Time) (int64, error) {
	return f.sum, f.err
}

type fakeBookingRepo struct {
	count int64
	err   error
}

func (f *fakeBookingRepo) Create(context.Context, *model.Booking) error { return nil }
func (f *fakeBookingRepo) GetByID(context.Context, uint) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) Update(context.Context, *model.Booking) error { return nil }
func (f *fakeBookingRepo) GetPendingByPhone(context.Context, string) (*model.Booking, error) {
	return nil, errors.New("booking not found")
}
func (f *fakeBookingRepo) ListExpiredPending(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListByHomeAndDate(context.Context, uint, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListUpcomingMissingLockCode(context.Context, time.Time, time.Duration) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListReadyToSendLockCode(context.Context, time.Time) ([]*model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) CountConfirmedBetween(context.Context, time.Time, time.Time) (int64, error) {
	return f.count, f.err
}

func TestSummaryReportsRevenueAndCount(t *testing.T) {
	uc := New(&fakePaymentRepo{sum: 4500000}, &fakeBookingRepo{count: 12})
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)

	resp, err := uc.Summary(context.Background(), from, to)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if resp.TotalRevenueVND != 4500000 {
		t.Errorf("TotalRevenueVND = %d, want 4500000", resp.TotalRevenueVND)
	}
	if resp.BookingCount != 12 {
		t.Errorf("BookingCount = %d, want 12", resp.BookingCount)
	}
	if !resp.From.Equal(from) || !resp.To.Equal(to) {
		t.Errorf("range = %v..%v, want %v..%v", resp.From, resp.To, from, to)
	}
}

func TestSummaryRejectsNonPositiveRange(t *testing.T) {
	uc := New(&fakePaymentRepo{}, &fakeBookingRepo{})
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := uc.Summary(context.Background(), day, day)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeValidation {
		t.Fatalf("Summary() error = %v, want CodeValidation", err)
	}
}

func TestSummarySurfacesRepoErrorAsAppErr(t *testing.T) {
	uc := New(&fakePaymentRepo{err: errors.New("db down")}, &fakeBookingRepo{})
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := uc.Summary(context.Background(), from, from.AddDate(0, 1, 0))
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.CodeInternal {
		t.Fatalf("Summary() error = %v, want CodeInternal", err)
	}
}
```

- [ ] **Step 7: Run the usecase tests**

Run: `go test ./usecase/overview/... -v`
Expected: PASS — 3 tests.

- [ ] **Step 8: Write `delivery/http/admin/overview_handler.go`** (`dateLayout` is already declared in `booking_handler.go`)

```go
package admin

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"
)

type OverviewHandler struct {
	uc        overviewuc.IUseCase
	handleErr HandleErrFunc
	handleOK  HandleOKFunc
}

func newOverviewHandler(uc overviewuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) *OverviewHandler {
	return &OverviewHandler{uc: uc, handleErr: handleErr, handleOK: handleOK}
}

func (h *OverviewHandler) summary(c echo.Context) error {
	now := time.Now()
	// Default window: month-to-date. `to` is exclusive, so it lands on tomorrow
	// to include everything booked for today.
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	if raw := c.QueryParam("from"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, now.Location())
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from, want YYYY-MM-DD"})
		}
		from = parsed
	}
	if raw := c.QueryParam("to"); raw != "" {
		parsed, err := time.ParseInLocation(dateLayout, raw, now.Location())
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to, want YYYY-MM-DD"})
		}
		to = parsed
	}

	resp, err := h.uc.Summary(c.Request().Context(), from, to)
	if err != nil {
		return h.handleErr(c, err)
	}
	return h.handleOK(c, resp)
}
```

- [ ] **Step 9: Write `delivery/http/admin/overview_route.go`**

```go
package admin

import (
	"github.com/labstack/echo/v4"

	overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"
)

func InitOverview(g *echo.Group, uc overviewuc.IUseCase, handleErr HandleErrFunc, handleOK HandleOKFunc) {
	h := newOverviewHandler(uc, handleErr, handleOK)
	g.GET("/overview", h.summary)
}
```

- [ ] **Step 10: Modify `delivery/http/http.go`** — add the parameter and mount the route

Add the import:

```go
	overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"
```

Add one field to the `Deps` struct (Task 6):

```go
	OverviewUC overviewuc.IUseCase
```

and mount it next to the other admin routes:

```go
	adminhttp.InitOverview(adminGroup, deps.OverviewUC, handleErr, handleOK)
```

- [ ] **Step 11: Modify `delivery/http/http_test.go`** — add the stub and pass it to `newTestServer()`

```go
type stubOverviewUC struct{}

func (stubOverviewUC) Summary(context.Context, time.Time, time.Time) (*presenter.OverviewResponse, error) {
	return nil, nil
}
```

Add `OverviewUC: stubOverviewUC{},` to the `Deps` literal inside `newTestServer()`.

- [ ] **Step 12: Modify `cmd/main.go`** — construct the usecase and pass it in

Add the import `overviewuc "github.com/johnquangdev/laverte-home/usecase/overview"`, then after the other usecases:

```go
	overviewUC := overviewuc.New(payments, bookings)
```

and add `OverviewUC: overviewUC,` to the `httpserver.Deps{...}` literal.

- [ ] **Step 13: Build and vet the whole module**

```bash
go build ./...
go vet ./...
```

Expected: no output from either command.

- [ ] **Step 14: Run the full unit-test suite without a database**

```bash
go test ./...
```

Expected: PASS for every package. The integration tests in `migrations` and `repository/booking` report `SKIP` because `TEST_DATABASE_URL` is unset — that is the intended signal, not a failure.

- [ ] **Step 15: Run the full suite against the docker-compose Postgres**

```bash
docker compose -f docker-compose.dev.yml up -d
TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test ./... -v
```

Expected: PASS, now including `TestMigrationsApplyCleanly`, the three exclusion-constraint tests in `repository/booking` (overlap rejected, adjacent allowed, cancelled ignored), and the payment-idempotency webhook tests.

- [ ] **Step 16: Manual smoke test against a locally running server**

Start the server in one terminal (`go run ./cmd`), then run the sequence below. Steps 2–3 hit admin endpoints, so the operator must first issue an access token for a user whose id is listed in `ADMIN_USER_IDS` — log in through `GET /api/v1/auth/google/login-url` → Google consent → `POST /api/v1/auth/google/callback` and copy `access_token` from the response into `$TOKEN`.

```bash
# 1. health
curl -s http://localhost:14000/health
# expect: {"ok":true}

# 2. pricing rule for category "nest" (hourly: 2 first hours 200k, 80k/extra hour)
curl -s -X POST http://localhost:14000/api/v1/admin/pricing-rules \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"category":"nest","rule_type":"hourly","base_hours":2,"base_price":200000,"extra_hour_price":80000,"effective_from":"2026-01-01T00:00:00Z","is_active":true}'
# expect: 200 with the created rule id

# 3. home in that category
curl -s -X POST http://localhost:14000/api/v1/admin/homes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Nest 1","category":"nest","address":"12 Le Loi","description":"smoke test"}'
# expect: 200 with the created home id — use it as $HOME_ID below

# 4. guest booking (public, no token)
curl -s -X POST http://localhost:14000/api/v1/bookings \
  -H 'Content-Type: application/json' \
  -d "{\"home_id\":$HOME_ID,\"customer_name\":\"Khach Smoke\",\"customer_phone\":\"0900000001\",\"start_time\":\"2026-08-01T10:00:00+07:00\",\"end_time\":\"2026-08-01T13:00:00+07:00\",\"booking_type\":\"hourly\"}"
# expect: 200, "status":"pending_payment", non-empty "qr_content", computed_price 280000

# 5. same phone again — anti-spam must return the SAME booking, not a new QR
curl -s -X POST http://localhost:14000/api/v1/bookings \
  -H 'Content-Type: application/json' \
  -d "{\"home_id\":$HOME_ID,\"customer_name\":\"Khach Smoke\",\"customer_phone\":\"0900000001\",\"start_time\":\"2026-08-01T15:00:00+07:00\",\"end_time\":\"2026-08-01T17:00:00+07:00\",\"booking_type\":\"hourly\"}"
# expect: 200 with the SAME booking id as step 4

# 6. admin sees it on the day view
curl -s "http://localhost:14000/api/v1/admin/bookings?home_id=$HOME_ID&date=2026-08-01" \
  -H "Authorization: Bearer $TOKEN"
# expect: 200 with exactly one booking, status pending_payment

# 7. revenue overview (nothing paid yet)
curl -s "http://localhost:14000/api/v1/admin/overview?from=2026-08-01&to=2026-09-01" \
  -H "Authorization: Bearer $TOKEN"
# expect: 200, total_revenue_vnd 0, booking_count 0 (the booking is not confirmed)
```

- [ ] **Step 17: Final commit**

```bash
go build ./...
go vet ./...
go test ./...
git add repository/booking presenter/overview.go usecase/overview delivery/http/admin/overview_handler.go delivery/http/admin/overview_route.go delivery/http/http.go delivery/http/http_test.go cmd/main.go
git commit -m "feat: admin revenue overview endpoint"
```
