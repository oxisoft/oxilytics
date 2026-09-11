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
- `internal/setup`: configured-store detection, setup mode gating (503), `/setup/*` API, connection tests with actionable errors.
- Screens: Setup, Store guides (content from 10-store-setup-guides.md), Settings–Stores.

### M3 — Sync engine (3–4 days)
- Migrations: products, apps, metric_days, reviews (+FTS), sync_runs, sync_run_logs, sync_checkpoints, ingested_objects.
- `internal/products`: CRUD, link/unlink with one-platform-per-product rule, name-normalised auto-link and fuzzy suggestions (tests).
- Engine: run lifecycle, per-store mutex, cancellation, progress, logs, checkpoints, full & delta for both stores, interrupted-run recovery.
- Scheduler (cron) driven by settings; backup + retention job.
- API: `/sync/*`, `/settings`. Screens: Sync, Sync run detail, Settings–General.

### M4 — Metrics & dashboard (3 days)
- `internal/metrics` queries scoped by product/platform/store/app; summary with per-platform split, series grouped by product|platform|store|app|country, day/week/month buckets, countries, products table; CSV export.
- Screens: Dashboard, Products, Product detail (all tabs except Reviews), Store apps. Chart components incl. platform stacking and donut.

### M5 — Reviews (1–2 days)
- `/reviews*` API with FTS, stats. Screens: Reviews list, detail modal, Product detail → Reviews tab.

### M6 — Release 1.0 (1–2 days)
- Multi-arch image workflow on tags, README with setup guide, `.env.example`, Caddyfile example.
- Run a real full sync against OxiSoft's accounts, fix parser gaps, tag `v1.0.0`.

Total ≈ 3 weeks of focused work.

## Definition of done (v1)
- Full sync of both stores completes on real OxiSoft accounts and the dashboard numbers match the store consoles for a sampled week (±1 day lag).
- Habit Observer (iOS + Android) shows as one product with correct per-platform and combined totals.
- A fresh container with no credentials shows the Setup screen; following the in-app guide for either store alone is enough to reach a successful full sync.
- Daily scheduled delta runs for 7 consecutive days without manual intervention.
- Container restarts mid-sync leave the DB consistent and the run marked `interrupted`.
- `go test -race` and frontend build are green in CI; image runs as non-root on amd64 and arm64.

## Open questions (need a decision)
1. **Apple ratings history** — approximated from reviews. Acceptable, or should we sample the iTunes lookup daily per country and store it as a proper time series (`rating_total_*` per country)? Recommendation: do the daily sampling; it's cheap.
2. **Product auto-link threshold** — exact normalised-name match auto-links; fuzzy (e.g. "Habit Observer" vs "Habit Observer – Tracker") only suggests. OK, or always require confirmation?
3. **macOS listings** — an App Store Connect app can be iOS+macOS under one id; v1 treats it as platform `ios` unless it is macOS-only. Fine for now?
4. **Single-app sync** ("Sync this app" button) — nice-to-have; drop from v1 if time is short.
5. **Alerts** (rating drop, crash spike, new 1★ review) via e-mail/Telegram — v2.
6. **Microsoft Store** — v2; the store abstraction (`store` column, per-store client + ingester) is designed so it's additive.
