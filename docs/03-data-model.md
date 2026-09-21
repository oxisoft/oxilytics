# 03 — Data model

SQLite, WAL mode, `foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL`.
Migrations: `internal/store/migrations/000N_name.sql` with goose `-- +goose Up/Down`
markers, applied at startup. Dates are stored as `TEXT` ISO-8601 (`YYYY-MM-DD` for
days, RFC 3339 UTC for timestamps). Booleans are `INTEGER 0/1`.

## Tables

### users
| column | type | notes |
|--------|------|-------|
| id | INTEGER PK | |
| email | TEXT UNIQUE NOT NULL | lower-cased |
| name | TEXT NOT NULL | |
| password_hash | TEXT NOT NULL | bcrypt |
| role | TEXT NOT NULL | `admin` / `viewer` |
| totp_secret | TEXT | NULL = disabled |
| totp_recovery | TEXT | JSON array of bcrypt hashes |
| disabled | INTEGER NOT NULL DEFAULT 0 | |
| created_at, updated_at, last_login_at | TEXT | |

### settings
| key TEXT PK | value TEXT NOT NULL | updated_at TEXT |

### products
| column | type | notes |
|--------|------|-------|
| id | INTEGER PK | |
| name | TEXT NOT NULL UNIQUE | "My Notes" |
| slug | TEXT NOT NULL UNIQUE | url-safe, used in routes |
| icon_url | TEXT | falls back to the first linked store app's icon |
| description | TEXT | |
| archived | INTEGER NOT NULL DEFAULT 0 | hidden from dashboard, data kept |
| created_at, updated_at | TEXT | |

A product has **at most one store app per platform** (enforced by a unique index on
`apps(product_id, platform) WHERE product_id IS NOT NULL`). Adding Windows later is a new
`store`/`platform` value, no schema change.

### apps (store apps)
| column | type | notes |
|--------|------|-------|
| id | INTEGER PK | |
| store | TEXT NOT NULL | `appstore` / `googleplay` |
| store_app_id | TEXT NOT NULL | ASC numeric app id / Android package name |
| name | TEXT NOT NULL | |
| bundle_id | TEXT | iOS bundle id (same as store_app_id on Android) |
| platform | TEXT | `ios`, `macos`, `android` … |
| icon_url | TEXT | |
| product_id | INTEGER FK products NULL | NULL = unassigned |
| suggested_product_id | INTEGER FK products NULL | auto-match suggestion awaiting admin confirmation |
| ignored_at | TEXT NULL | NULL = active. Set → excluded from sync and from every non-admin query; data retained |
| ignored_by | INTEGER FK users NULL | |
| ignored_reason | TEXT | free text shown in the Ignored list ("old test build") |
| rating_avg | REAL | current store-wide average, refreshed each sync (Apple: iTunes lookup; Google: latest `Total Average Rating`) |
| rating_count | INTEGER | Apple only (`userRatingCount`); NULL on Google |
| rating_updated_at | TEXT | |
| first_seen_at, last_synced_at | TEXT | |
| UNIQUE(store, store_app_id) | | |
| UNIQUE(product_id, platform) WHERE product_id IS NOT NULL | | |

Ignoring an app that is linked to a product **unlinks it first** (a product must not show a ghost platform). Every store/metrics/reviews query joins `apps` with `ignored_at IS NULL` unless the caller explicitly asks for ignored rows (admin endpoints only). Index on `apps(ignored_at)`.

All metric and review rows hang off the **store app**; product-level numbers are
`SUM`/`AVG` over `apps.product_id` at query time (indexes on `apps(product_id)` and
`metric_days(app_id, day)` make that cheap). Re-linking a store app to another product
never moves data — it is just a foreign-key change.

### metric_days
One row per app × day × country. `country` is ISO-3166 alpha-2 or `ZZ` for unknown.
Totals are computed by summing over countries; the `*_overview` Google Play file and
the un-segmented Apple report are stored under `country='*'` so that daily totals are
exact even where the per-country data is sampled or missing.

| column | type | notes |
|--------|------|-------|
| app_id | INTEGER FK apps | |
| day | TEXT | `YYYY-MM-DD` |
| country | TEXT | `*` = total row |
| downloads | INTEGER | Apple: first-time downloads; Google: `Daily User Installs` |
| redownloads | INTEGER | Apple only |
| updates | INTEGER | Apple: updates; Google: `Daily Device Upgrades` (if present) |
| uninstalls | INTEGER | Apple: deletions; Google: `Daily User Uninstalls` |
| active_devices | INTEGER | Google `Active Device Installs` (snapshot, not summable) |
| crashes | INTEGER | Apple `App Crashes` count; Google `Daily Crashes` |
| anrs | INTEGER | Google only |
| source_updated_at | TEXT | when the store file/report was produced |
| PRIMARY KEY (app_id, day, country) | | |

Index: `(day)`, `(app_id, day)`.

### reviews
| column | type | notes |
|--------|------|-------|
| id | INTEGER PK | |
| app_id | INTEGER FK | |
| store_review_id | TEXT NOT NULL | ASC review id / Google `reviewId` |
| rating | INTEGER NOT NULL | 1–5 |
| title | TEXT | Apple only |
| body | TEXT | |
| author | TEXT | nickname |
| country | TEXT | territory / reviewer language-country |
| language | TEXT | |
| app_version | TEXT | |
| device | TEXT | Google only |
| created_at | TEXT NOT NULL | store timestamp |
| edited_at | TEXT | Google `lastModified` |
| developer_reply | TEXT | |
| developer_replied_at | TEXT | |
| fetched_at | TEXT | |
| UNIQUE(app_id, store_review_id) | | |

Index: `(app_id, created_at DESC)`, `(created_at DESC)`, `(rating)`.
FTS5 virtual table `reviews_fts(title, body, content='reviews')` for search, kept in sync by triggers.

### sync_runs
| column | type | notes |
|--------|------|-------|
| id | INTEGER PK | |
| store | TEXT NOT NULL | |
| mode | TEXT NOT NULL | `full` / `delta` |
| trigger | TEXT NOT NULL | `manual` / `schedule` |
| requested_by | INTEGER FK users | NULL for schedule |
| status | TEXT NOT NULL | `queued` `running` `succeeded` `failed` `cancelled` `interrupted` |
| started_at, finished_at | TEXT | |
| range_from, range_to | TEXT | days covered |
| apps_total, apps_done | INTEGER | progress |
| rows_metrics, rows_reviews | INTEGER | counters |
| error | TEXT | last error message |
| stats | TEXT | JSON: per-source counts, API calls, bytes |

### sync_run_logs
| id | run_id FK | ts | level | app_id NULL | message |

Trimmed to 2 000 lines per run.

### sync_checkpoints
Per (store, source, app) "what have I already got".

| column | notes |
|--------|-------|
| store, source, app_id | PK; `source` ∈ `metrics`, `reviews`, `crashes` |
| cursor | TEXT: last day ingested (`YYYY-MM-DD`), or last review timestamp / GCS object generation |
| updated_at | |

### ingested_objects (Google Play)
| store | object_name PK | generation | md5 | ingested_at |

Lets a delta run skip bucket objects that did not change.

### api_tokens
| column | notes |
|--------|-------|
| id | PK |
| user_id | FK → `users(id)` `ON DELETE CASCADE`; deleting a user revokes their tokens |
| name | what the token is for, shown in the UI |
| token_hash | SHA-256 of the plaintext, UNIQUE; plaintext is never stored |
| prefix | first characters (`oxi_` + 6) for display, so a token is recognisable without being exposed |
| created_at, last_used_at, revoked_at | `revoked_at IS NULL` means active; revoked rows are kept as an audit trail |
| can_run_sync | opt-in capability, `NOT NULL DEFAULT 0`; permits only `POST /api/sync/runs`. Fixed at creation — no code path updates it, so a token's powers cannot change after it is issued |

SHA-256 rather than bcrypt: the token is 256 bits of CSPRNG output with no
guessable structure, and it is verified on every request where a deliberately
slow hash would be a DoS vector. `foreign_keys` is ON in the DSN, so the
cascade actually fires.

## Retention

`metrics.retention_days > 0` → a nightly job deletes `metric_days` older than N days.
Reviews are never auto-deleted. Sync runs older than 90 days are pruned with their logs.

## Sizing

10 apps × 2 stores × 5 years × ~60 countries ≈ 2.2 M metric rows ≈ 250 MB. Fine for SQLite.
