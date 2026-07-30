# Next steps

State as of branch `feat/laverte-home-backend`, 75 commits, all 18 planned tasks plus
a whole-branch review and its fix round complete. `go build`/`go vet` clean, both lint
configs 0 issues, no `//nolint` anywhere, full suite (`TEST_DATABASE_URL` set) 27
packages ok / FAIL=0. The branch is merge-safe; nothing below is a merge blocker,
but several are worth fixing before real money and real guests hit it.

## Before real money flows

- **Unreconcilable money has no admin alert.** `usecase/billing/webhook.go`: a
  transfer that lands after the hold expired gets HTTP 410; an amount mismatch gets
  400. Both leave the payment row `pending` (later swept to `expired`) with the
  money already in the bank, and nobody is told. Add an admin notification on both
  branches, the same shape as `AdminLockCodeMissing`.
- **`Payment` has no refund status or concept.** If a booking is cancelled after its
  payment settled, there's no way to record that the money was returned. Low
  priority while cancellations after payment are rare and manual, but the revenue
  overview (`usecase/overview`) will overcount the moment one happens.

## Before deploying anywhere

- **No Dockerfile.** `docker-compose.dev.yml` only runs Postgres + Redis; there's no
  containerized app service, no healthchecks, and no way to actually deploy this
  branch today.
- **No graceful shutdown.** `cmd/main.go` has no signal handling; `srv.Start()`
  blocks and its failure path is `log.Fatal` → `os.Exit(1)`, which skips deferred
  calls including `job.Stop()`. A SIGTERM mid-sweep (any restart or redeploy) can
  strand a booking between `MarkPaidIfPending` and its confirm, or strand a claimed
  door-code send with `ReleaseLockCodeSend` never called — a guest at a locked door
  with no code and nothing retrying it. `signal.NotifyContext` + `echo.Shutdown` +
  `job.Stop()` is roughly 15 lines.

## Security / correctness, smaller blast radius

- **Logout doesn't revoke the refresh token.** `usecase/auth.Logout` blacklists only
  the access token id; `refresh_tokens.revoked_at` is never touched, so a stolen or
  shared-device refresh token still mints access tokens for up to
  `JWT_REFRESH_TTL_DAYS` (default 30) after logout. The revocation machinery
  (`RevokeFamily`, family-reuse detection) already exists — it's just not wired to
  logout. Needs the refresh token (or its family, resolvable from the access token's
  claim) threaded into the logout call.
- **Changing a door code after it was sent doesn't clear `lock_code_sent_at`.**
  `usecase/bookingadmin.SetLockCode` writes only the code column. If the physical
  lock is reprogrammed after the sweep already delivered the old code,
  `SendLockCode`'s claim finds `lock_code_sent_at` non-NULL and returns "success"
  having sent nothing — the guest keeps the stale code. `SetDoorLockCode` (or a
  variant of it) should clear the timestamp when the value actually changes.

## Spec gaps

- No `/me` route (in the original design's route list, no task ever built it).
- `blocked_slots` has no DB-level exclusion constraint — `HasOverlap` is a
  check-then-insert race, same shape the `bookings` table's constraint exists to
  close. Low likelihood at 3 homes / 1-2 admins, but worth the same fix for
  symmetry if it's ever cheap to add.
- No sanity check that `ExtraHourPrice < BasePrice / BaseHours` when a pricing rule
  is created — a fat-fingered rule can make a longer stay cost more than several
  short ones, which contradicts the product's own pricing promise.

## Cosmetic / cleanup, no urgency

- `delivery/http/http.go`'s `trimSpace` for `CORS_ORIGINS` strips only ASCII space,
  not tabs/newlines — a multi-line `.env` value could silently mismatch an origin.
- `SEPAY_API_KEY` is declared in config and `.env.example` but read nowhere;
  `CreateQR` doesn't use it. Either wire it in or drop it so it stops looking
  load-bearing.
- Exported identifiers in `config/` and `errors/` mostly lack doc comments.

## Where to look for more detail

- `.superpowers/sdd/2026-07-29-laverte-home-backend/progress.md` — the full task
  ledger: every review finding, every fix round, every deferral with its reasoning.
- `docs/superpowers/specs/2026-07-29-laverte-home-backend-design.md` — the approved
  design spec, for anything that looks like a gap against the original intent.
