# laverte-core

Go backend for a homestay booking system: 3 properties in 2 tiers (`home`, `nest`).
This repo is the API only — the guest site and the one admin UI live in
`laverte-home-frontend` (`/book`, `/admin/*`), which calls this API through its
own server-side proxy.
Guests book by the hour, overnight, or by the day, **without an account** — name and
phone only — and **pay before the slot is held**. Admins use Google OAuth to manage
pricing, walk-in bookings, blocked slots, door lock codes, and a revenue overview.

## Architecture

Clean architecture, one direction of dependency: `delivery → usecase → repository →
model`. Each usecase package declares its own consumer-sized interface
(`repository/x/interface.go`) and is constructed with `New(...)` taking every
dependency explicitly — no globals, no service locator. `delivery/http/http.go`'s
`NewServer(cfg, log, deps Deps)` takes a single `Deps` struct; every new usecase adds
one field there and one line in `cmd/main.go`.

Stack: Go 1.25, Echo v4, GORM + `jackc/pgx/v5`, Redis (`go-redis/v9`),
`rubenv/sql-migrate`, `golang-jwt/jwt/v5`, `golang.org/x/oauth2`,
`google.golang.org/api/calendar/v3`, `robfig/cron/v3`, `go.uber.org/zap`,
`kelseyhightower/envconfig`, `go-playground/validator/v10`.

## The two invariants everything else serves

1. **No double-booking.** The only defence is a Postgres `EXCLUDE USING gist` over
   `(home_id, tstzrange(start_time, end_time))`
   (`migrations/0004_bookings.sql`), partial on
   `status IN ('pending_payment','confirmed')`. Any status outside that set
   (`expired`, `cancelled`, `completed`, `no_show`) releases the slot. **Never write to
   `bookings` except through the guarded single-column methods in
   `repository/booking/interface.go`** — `ConfirmIfPending`, `CancelIfNotTerminal`,
   `CompleteIfConfirmed`, `NoShowIfConfirmed`, `ReleaseHoldIfPending`,
   `SetPaymentID`, `SetCalendarEventID`, `SetDoorLockCode`, `ClaimLockCodeSend`,
   `ReleaseLockCodeSend`. **There is no `Update` method on this interface — it was
   deleted on purpose.** GORM's `Save()` rewrites every column from an in-memory
   snapshot; three Criticals during development were exactly that pattern reverting a
   concurrent write (a cancelled booking resurrected to `confirmed`, a paid booking
   losing its `payment_id`). If you need a new booking mutation, add a new guarded
   method in this shape — `Where("id = ? AND <precondition>", id)`, single-column
   `Update`, return `RowsAffected > 0` — never a struct-based `Save`.
2. **The door lock code is a physical-access credential** — it must reach exactly one
   guest, exactly once, and only while the booking is `confirmed`. `ClaimLockCodeSend`
   is the sole gate (conditional `UPDATE ... WHERE lock_code_sent_at IS NULL AND
   status = 'confirmed'`); a failed send calls `ReleaseLockCodeSend` so a retry can
   still deliver.

## Booking lifecycle

`pending_payment` (with `expires_at`) → SePay webhook confirms → `confirmed` →
`completed` / `no_show`, or `cancelled` from either of the first two states.
`completed` and `no_show` are terminal — no path may re-enter them into
`cancelled`, because their payment is earned revenue. A refund is a record, not a
transfer (money goes back by hand): `repository/ledger.RefundIfStayEnded` flips a
`paid` payment to `refunded` only while its booking is `cancelled` or `expired`,
and revenue sums only `paid`. A bank transfer that settles no booking (unreadable
memo, unknown booking, hold already expired, wrong amount) is kept in
`unmatched_transfers`, deduplicated on the provider's transaction id, and alerts
the admin once. A cron sweep (`usecase/bookingjobs`) expires abandoned
holds, alerts the admin when a stay is about to start with no door code, and
auto-sends the code at check-in — all through the guarded writes above.

## Testing

- Unit tests use hand-written fakes per package (no mock framework). **A fake that
  returns a constant where the real query has a `WHERE` predicate is a shipped bug
  waiting to happen** — model the predicate, not just the happy path. `GetByID` fakes
  return a **copy**, mirroring GORM's `First`, so a test can simulate a concurrent
  writer landing between a read and a write-back (see `afterGetByID`/`beforeClaim`
  hooks in `usecase/bookingadmin/usecase_test.go`).
- Integration tests (`repository/*/pg_integration_test.go`) need
  `TEST_DATABASE_URL` set and the dev stack running. `internal/testdb.New(t, suffix)`
  gives each test package its own throwaway database — `go test` runs packages in
  parallel, and a shared database means one package's cleanup drops another's
  fixtures mid-run.
- **Every guard needs a test that can fail.** Before trusting a new test, mutate the
  code it claims to cover and confirm the test actually goes red, then revert. This
  project shipped several tests that passed with the guard deleted (a call-counter
  missing, a fake aliasing the live struct instead of copying it) — each was only
  caught by an adversarial re-check, never by the original author.

## Commands

```bash
docker compose -f docker-compose.dev.yml up -d   # Postgres :55432, Redis :6380
go run ./cmd/migrate                              # migrate to head
go run ./cmd                                      # start the server (:14000 by default)

go build ./... && go vet ./...
golangci-lint run --config ~/.claude/golangci-default.yml
golangci-lint run --config ~/.claude/golangci-naming.yml   # staticcheck ST1003 — no underscored package names

go test ./...                                     # unit tests only
TEST_DATABASE_URL=postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable go test -count=1 ./...
```

Copy `.env.example` to `.env` and fill in secrets before running. `SEPAY_BANK_ACCOUNT`,
`SEPAY_BANK_CODE`, and `SEPAY_WEBHOOK_SECRET` are required — `config.Validate()`
refuses to start without them, because there is no safe degraded mode on the money
path (unlike the notifier/Calendar adapters, which warn and no-op when half
configured).

## Conventions

- Errors: sentinels (`ErrSlotConflict`, `ErrDuplicateExternalRef`, ...) from SQLSTATE
  translation in `repository/*/pg.go`; usecases wrap them via the `apperr` helpers
  (`apperr.Validation`, `apperr.NotFound`, `apperr.Internal`, ...) so the HTTP layer's
  shared `handleErr` maps them correctly. A bare `errors.New` escaping a usecase
  becomes an HTTP 500 — that is `handleErr`'s fallback, not a feature.
- Phone numbers: always pass through `model.NormalizeVNPhone` before use as a lookup
  key (booking re-use checks, rate limiting) — one canonical spelling per number.
- Money math: integer `time.Duration` arithmetic only, never `float64` — see the
  comments in `usecase/pricing/usecase.go` for why the rounding behaviour matters on
  a customer-facing price.
- Admin routes live under `authed.Group("/admin", requireAdmin)`
  (`delivery/http/http.go`) — both JWT and the admin check are enforced at
  registration, not per-handler; verify new routes are mounted there, not on `authed`
  directly.
- Overnight windows are wall-clock rules in the business zone: `usecase/pricing`
  reads a booking's start in `Deps.Location`, never in whatever offset the client
  serialised it with.
- Admin-only reads and corrections that the settlement path never needs live in
  their own repositories (`ledger`, `report`, `unmatchedtransfer`) so the fakes of
  `repository/booking` and `repository/payment` don't grow with every screen.
- Dates from admin query params (`?date=`, `?from=`, `?to=`) parse in
  `cfg.AppTimeZone` (default `Asia/Ho_Chi_Minh`), resolved once at boot into
  `Deps.Location` — never `time.Now().Location()`, which is host-TZ-dependent.

## Comments

Explain *why* — an invariant, a race, an external constraint, a pitfall — never
*what* the adjacent line already says. Don't name specific infra in logic comments.
Don't defend your own change ("we now do X instead of Y"). See
`~/.claude/CLAUDE.md` for the full house rule; it's enforced on every edit by a hook,
not just at review time.

## Known gaps (not yet fixed, tracked for a follow-up PR)

See `docs/NEXT-STEPS.md`.
