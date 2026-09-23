# Next steps

State as of branch `feat/laverte-home-backend`, 75 commits, all 18 planned tasks plus
a whole-branch review and its fix round complete. `go build`/`go vet` clean, both lint
configs 0 issues, no `//nolint` anywhere, full suite (`TEST_DATABASE_URL` set) 27
packages ok / FAIL=0. The branch is merge-safe; nothing below is a merge blocker,
but several are worth fixing before real money and real guests hit it.

## Before real money flows

- **Refunds are ledger records only.** `POST /admin/payments/:id/refund` marks a
  paid payment of a cancelled/expired stay as `refunded` with the admin's note;
  the money itself is returned by hand. Revenue drops out of the month it was
  *paid* in, not the month it was refunded — acceptable while refunds are rare,
  wrong for a cash-basis report once they are not.
- **Unmatched transfers can be resolved, not re-assigned.** An admin records how
  a stray transfer was handled; there is no "attach this money to booking #N"
  action, because re-confirming an expired hold needs a fresh overlap check and
  a decision about price differences.
- **Client IP in production.** The frontend's server calls `be.laverte.vn`
  through the CDN, so the backend's per-IP limiter only sees the forwarded
  address the BFF supplies (`CF-Connecting-IP` first). Pointing the frontend's
  `API_ROOT` at the API over the shared Docker network (with a network alias on
  the core's `api` service, deployed before the frontend switches) would skip the
  CDN hop entirely; the backend should then trust `X-Forwarded-For` only from
  that network.

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
  The admin roster screen infers superadmin from a 403 on `GET /admin/admins`.
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
