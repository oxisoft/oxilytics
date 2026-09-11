# 09 — Roadmap & open questions

## Milestones

Each milestone ends with `go vet`, `go test`, `npm run build` green and a runnable binary.

### M0 — Skeleton (1–2 days)
- Repo layout, `go.mod`, chi server, slog, config loader, `/api/health`, `/api/version`.
- SQLite open with pragmas, writer/reader split, goose embedded migrations, `0001_init.sql` (users, settings).
- Svelte 5 + Vite + Tailwind 4 + router + i18n scaffold, `web/embed.go`, SPA fallback.
- Dockerfile, compose, Makefile, CI (vet/test/build).

### M1 — Auth & users (2 days)
- Sessions, login/logout, bcrypt, rate limit, CSRF header check, TOTP + recovery codes.
- Bootstrap admin from env. Users CRUD (admin). Profile screen. Permissions package with tests.
- Screens: Login, TOTP, Profile, Settings–Users, About, top bar/footer.

### M2 — Store clients (3–4 days)
- `appstoreconnect`: JWT, apps, customerReviews paging, analytics report request/instances/segments download, iTunes lookup. Fixture-based tests.
- `googleplay`: service-account token, GCS list/download (UTF-16 decode), CSV parsers for installs/ratings/crashes/reviews, `reviews.list`. Fixture-based tests.
- "Test connection" endpoint + Settings–Stores screen.

### M3 — Sync engine (3–4 days)
- Migrations: apps, metric_days, reviews (+FTS), sync_runs, sync_run_logs, sync_checkpoints, ingested_objects.
- Engine: run lifecycle, per-store mutex, cancellation, progress, logs, checkpoints, full & delta for both stores, interrupted-run recovery.
- Scheduler (cron) driven by settings; backup + retention job.
- API: `/sync/*`, `/settings`. Screens: Sync, Sync run detail, Settings–General.

### M4 — Metrics & dashboard (3 days)
- `internal/metrics` queries (summary, series with day/week/month bucketing, countries), CSV export.
- Screens: Dashboard, Apps, App detail (all tabs except Reviews). Chart components.

### M5 — Reviews (1–2 days)
- `/reviews*` API with FTS, stats. Screens: Reviews list, detail modal, App detail → Reviews tab.

### M6 — Release 1.0 (1–2 days)
- Multi-arch image workflow on tags, README with setup guide, `.env.example`, Caddyfile example.
- Run a real full sync against OxiSoft's accounts, fix parser gaps, tag `v1.0.0`.

Total ≈ 3 weeks of focused work.

## Definition of done (v1)
- Full sync of both stores completes on real OxiSoft accounts and the dashboard numbers match the store consoles for a sampled week (±1 day lag).
- Daily scheduled delta runs for 7 consecutive days without manual intervention.
- Container restarts mid-sync leave the DB consistent and the run marked `interrupted`.
- `go test -race` and frontend build are green in CI; image runs as non-root on amd64 and arm64.

## Open questions (need a decision)
1. **Google Play app name & icon** — no cheap official API for the store listing. Options: (a) admin sets them manually in Apps (default plan), (b) Android Publisher `edits.details/listings` (needs an edit per read, clumsy but official), (c) scrape the public Play page (fragile). Recommendation: (a) now, (b) later.
2. **Apple ratings history** — approximated from reviews. Acceptable, or should we sample the iTunes lookup daily per country and store it as a proper time series (`rating_total_*` per country)? Recommendation: do the daily sampling; it's cheap.
3. **Product key linking** — manual field vs auto-match on name. Recommendation: manual, with a "suggest" button that matches on normalised name.
4. **Single-app sync** ("Sync this app" button) — nice-to-have; drop from v1 if time is short.
5. **Alerts** (rating drop, crash spike, new 1★ review) via e-mail/Telegram — v2.
6. **Microsoft Store** — v2; the store abstraction (`store` column, per-store client + ingester) is designed so it's additive.
