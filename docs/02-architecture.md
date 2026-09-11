# 02 — Architecture

## One process, one origin

```
                ┌──────────────────────────────────────────────────────┐
 browser ──TLS──▶ reverse proxy (Caddy/Traefik/nginx, not part of repo) │
                └───────────────┬──────────────────────────────────────┘
                                │ http://127.0.0.1:8080
                ┌───────────────▼──────────────────────────────────────┐
                │ oxilytics (Go binary)                                 │
                │  chi router                                            │
                │   /api/*        → httpapi handlers (JSON)              │
                │   /*            → embedded web/dist, fallback index.html│
                │  domain services: auth, apps, metrics, reviews, sync   │
                │  store (SQL)  ── modernc.org/sqlite ──▶ /data/oxilytics.db (WAL)
                │  cron: daily sync, nightly backup                      │
                │  store clients: appstoreconnect, googleplay            │
                └────────────────────────────────────────────────────────┘
                          │ HTTPS                       │ HTTPS
                          ▼                             ▼
                api.appstoreconnect.apple.com   storage.googleapis.com / androidpublisher.googleapis.com
```

No CORS, no separate frontend server, no external queue. State that must survive a
restart lives in SQLite; the in-memory parts (running sync, progress) are rebuilt from
the `sync_runs` table on startup (runs left `running` are marked `interrupted`).

## Layers

```
httpapi  →  domain services  →  store (SQL)
   ▲             │
   │             └── storeclient (appstoreconnect / googleplay)  — HTTP to the stores
 permissions (pure functions, no I/O)
```

Rules:
- Handlers: decode + validate input, call `permissions`, call one service, encode output. **No SQL in handlers.**
- Store: plain SQL, returns models. **Knows nothing about HTTP.** One `*sql.DB` for the writer (`MaxOpenConns=1`), one pool for readers (`MaxOpenConns=4`).
- Domain services: pure where possible. The sync service is the only long-running one.
- Store clients: thin, typed, testable with recorded fixtures; no business logic (parsing CSV/TSV is theirs, deciding what a "download" is belongs to `internal/sync`).

## Repository layout

```
/cmd/server/main.go            config → open DB → goose up → wire services → cron → HTTP
/internal/config               env vars → Config struct (validated at start)
/internal/store                sqlite.go (open, pragmas, writer/reader), *_repo.go per table group
/internal/store/migrations     000N_*.sql, embedded with go:embed
/internal/models               App, MetricDay, Review, SyncRun, User, Setting …
/internal/auth                 sessions (gorilla), bcrypt, TOTP, middleware RequireUser/RequireAdmin
/internal/permissions          Can(user, action, resource) bool — pure
/internal/httpapi              router.go, handlers_*.go, respond.go, errors.go
/internal/products             products + store-app linking, auto-match suggestions
/internal/apps                 store-app catalogue service
/internal/metrics              aggregation queries for dashboard (series, totals, breakdowns)
/internal/reviews              review listing/search service
/internal/sync                 engine: runs, modes, checkpoints, scheduler, progress
/internal/sync/appstore        ingestion for App Store (uses storeclient/appstoreconnect)
/internal/sync/googleplay      ingestion for Google Play
/internal/storeclient/appstoreconnect   JWT auth, analytics reports, reviews, apps
/internal/storeclient/googleplay        service-account auth, GCS bucket reader, reviews API
/internal/settings             typed access to the settings table
/internal/setup                setup-mode detection, per-store guide metadata, connection tests
/internal/backup               VACUUM INTO + rotation
/internal/version              Version, Commit, BuildTime (ldflags)
/web                           Svelte 5 + Vite + Tailwind 4 + svelte-spa-router + svelte-i18n + chart.js
/web/embed.go                  //go:embed all:dist → fs.FS
/web/src/lib/api.js            fetch wrapper (JSON, CSRF header, 401 → login)
/web/src/locales/en.json
/deploy/docker-compose.yml     reference deployment
/deploy/Caddyfile.example
/Dockerfile
/.github/workflows/ci.yml      vet + test + build on every push; multi-arch image on tags
/docs                          this folder
```

## Configuration (env vars)

All prefixed `OXI_`. Read once at start; the process refuses to start on invalid config.

| Var | Default | Notes |
|-----|---------|-------|
| `OXI_ADDR` | `127.0.0.1:8080` | listen address |
| `OXI_DB_PATH` | `/data/oxilytics.db` | SQLite file (WAL + shm next to it) |
| `OXI_SESSION_KEY` | — required | 64 hex chars; cookie auth + encryption key |
| `OXI_BASE_URL` | `http://localhost:8080` | used for cookie `Secure` flag and links |
| `OXI_TZ` | `Europe/Warsaw` | timezone for "run at 09:00" and day bucketing in the UI |
| `OXI_LOG_LEVEL` | `info` | slog level |
| `OXI_BOOTSTRAP_ADMIN_EMAIL` / `OXI_BOOTSTRAP_ADMIN_PASSWORD` | — | created only when the `users` table is empty |
| `OXI_ASC_KEY_ID` | — | App Store Connect API key id |
| `OXI_ASC_ISSUER_ID` | — | |
| `OXI_ASC_KEY_FILE` | `/secrets/AuthKey.p8` | |
| `OXI_GPLAY_SA_FILE` | `/secrets/gplay-sa.json` | service account JSON |
| `OXI_GPLAY_BUCKET` | — | `pubsite_prod_rev_XXXXXXXXXXXXXXXX` from Play Console → Download reports |
| `OXI_BACKUP_DIR` | `/data/backups` | empty string disables backups |
| `OXI_BACKUP_KEEP` | `14` | number of daily backups kept |

A store is **configured** when all its variables are set and the files exist and parse
(the `.p8` is a valid EC key, the service-account JSON has `client_email`/`private_key`).
Validation happens at start-up and the result is exposed on `/api/setup/status`.

- **0 stores configured → setup mode** (see 01-overview): the API serves only `/auth/*`,
  `/me`, `/setup/*`, `/version`, `/health`; everything else answers `503 setup_required`.
  The SPA routes every screen to `#/setup`.
- 1 store configured → normal mode; the other store's card shows "Not configured — set up" linking to its guide.
- Configuration is re-checked only at start-up (credentials are files/env, so a restart is needed anyway).

## Runtime settings (DB `settings` table)

Editable by admins in the UI, applied without restart:

| Key | Default | Meaning |
|-----|---------|---------|
| `sync.schedule.enabled` | `true` | daily automatic sync |
| `sync.schedule.time` | `06:30` | local time (`OXI_TZ`) |
| `sync.schedule.stores` | `appstore,googleplay` | which stores the daily run covers |
| `sync.delta.overlap_days` | `3` | delta runs re-fetch this many days back (stores restate recent days) |
| `metrics.retention_days` | `0` | 0 = keep forever |
| `ui.default_range_days` | `30` | initial dashboard range |
| `products.auto_link` | `true` | when sync discovers a new store app, auto-link it to a product whose normalised name matches exactly; otherwise leave unassigned with a suggestion |

## Auth

- `gorilla/sessions` cookie store: encrypted, `HttpOnly`, `SameSite=Lax`, `Secure` when base URL is https. Session lifetime 30 days, sliding.
- Password: bcrypt cost 12. Login rate-limited per IP (in-memory token bucket).
- TOTP optional per user (`pquerna/otp`); login is two steps when enabled. Recovery codes: 8, bcrypt-hashed.
- CSRF: the SPA sends `X-Requested-With: fetch` on every mutating request; the API rejects mutating requests without it (cookie is `SameSite=Lax`, so this is sufficient for a same-origin SPA).
- Unauthenticated `/api/*` → `401`; the SPA redirects to `#/login`.

## Permissions (`internal/permissions`)

Pure table, no I/O:

| Action | admin | viewer |
|--------|-------|--------|
| view dashboard / products / apps / metrics / reviews | ✔ | ✔ |
| create / edit products, link & unlink store apps | ✔ | ✖ |
| view setup guide & status | ✔ | ✔ |
| run store connection test | ✔ | ✖ |
| view sync status & history | ✔ | ✔ |
| start / cancel sync | ✔ | ✖ |
| edit settings | ✔ | ✖ |
| manage users | ✔ | ✖ |
| manage own profile / TOTP | ✔ | ✔ |

The server filters responses: e.g. `GET /api/users` is admin-only; `GET /api/me` returns the caller's own record.

## Versioning

`-ldflags "-X internal/version.Version=… -X …Commit=… -X …BuildTime=…"`; exposed by
`GET /api/version` and shown in the UI footer and the About dialog.

## Logging

`log/slog`, JSON to stdout. Every request logged with method, path, status, duration,
user id. Sync logs carry `run_id`, `store`, `app_id` attributes; the sync engine also
appends the important ones to `sync_run_logs` so the UI can show them.
