# 06 — REST API

Base: `/api`. JSON in/out. Errors: `{"error":{"code":"…","message":"…","fields":{…}}}`.
Status codes: 200/201/204, 400 validation, 401 unauthenticated, 403 forbidden,
404, 409 conflict, 429, 500. Mutating requests require header `X-Requested-With: fetch`.
Dates in query params are `YYYY-MM-DD` in `OXI_TZ`.

## Auth / session
| Method | Path | Role | Notes |
|--------|------|------|-------|
| POST | `/auth/login` | — | `{email,password}` → `200 {user}` or `202 {totp_required:true}` |
| POST | `/auth/totp` | — | `{code}` completes a pending TOTP login |
| POST | `/auth/logout` | any | |
| GET | `/me` | any | current user |
| PUT | `/me` | any | name |
| PUT | `/me/password` | any | `{current,new}` |
| POST | `/me/totp/setup` | any | returns secret + otpauth URL (QR rendered client-side) |
| POST | `/me/totp/enable` | any | `{code}` → recovery codes (shown once) |
| DELETE | `/me/totp` | any | `{password}` |

## API tokens

Tokens let scripts and integrations read analytics without a browser session.
They are **read-only by default**: the router allows only `GET`/`HEAD`/`OPTIONS`
for token-authenticated requests, so a token cannot write even when its owner is
an admin, and a write endpoint added later is denied by default.

A token may be granted one narrow capability at creation, and only that one:

| Capability | Grants | Does not grant |
|---|---|---|
| `can_run_sync` | `POST /api/sync/runs` — start a sync that is already configured | cancelling or resetting a run, changing any setting, anything else |

Capabilities are chosen when the token is created and are **fixed for its
lifetime** — there is no endpoint to edit them, so what a token can do is
knowable from its creation record. A capability can never exceed its owner:
creating a sync-capable token requires the `run_sync` permission, and the sync
route re-checks that permission on every call.

Use the `Authorization` header. Tokens in query strings are not accepted, because
they leak into access logs, browser history and `Referer` headers.

```
curl -H "Authorization: Bearer ***" https://analytics.example.com/api/products

curl -X POST -H "Authorization: Bearer ***" -H "Content-Type: application/json" \
  -d '{"store":"appstore","mode":"delta"}' \
  https://analytics.example.com/api/sync/runs
```

The `X-Requested-With` CSRF header is **not** required for token requests. That
header exists because browsers attach cookies automatically; a Bearer token
never is, so a hostile page cannot forge an authenticated token call.

- Owned by the user who created them; deleting or disabling that user revokes them.
- Only a SHA-256 hash is stored. The plaintext is shown once at creation and cannot be recovered.
- `last_used_at` is recorded (at most once a minute per token) so stale tokens are visible.
- Tokens never expire; revoke them when they are no longer needed.
- Managed from **My profile → API tokens**, session-only: a token cannot mint or revoke tokens.

| Method | Path | Role | Notes |
|--------|------|------|-------|
| GET | `/me/tokens` | any (session only) | list own tokens; never returns plaintext |
| POST | `/me/tokens` | any (session only) | `{name, can_run_sync?}` → `201 {…, token}` — the only time plaintext is returned |
| DELETE | `/me/tokens/{id}` | any (session only) | revoke; takes effect immediately |

Failure modes: `401 invalid_token` (unknown, revoked, or owner disabled),
`403 read_only_token` (write attempted with a token that lacks the capability),
`403 session_required` (token used on a session-only endpoint),
`422` on creation when requesting a capability the owner does not have.

## Users (admin)
| GET | `/users` | list |
| POST | `/users` | `{email,name,role,password}` |
| PUT | `/users/{id}` | name, role, disabled |
| PUT | `/users/{id}/password` | reset |
| DELETE | `/users/{id}` | cannot delete self / last admin |

## Setup
| GET | `/setup/status` | any | `{setup_required, stores:{appstore:{configured, checks:[{name,ok,detail}]}, googleplay:{…}}}` — checks: env var set, file readable, key parses, sa e-mail, bucket name |
| POST | `/setup/test/{store}` | admin | live connection test: ASC `GET /v1/apps?limit=1`; Play: list bucket with `maxResults=1` + `reviews.list` on first package. Returns `{ok, steps:[{name,ok,detail}]}` with actionable error text (e.g. "403: the service account is not invited in Play Console") |
| GET | `/setup/guide/{store}` | any | guide metadata (env var names, current values masked, links) — the guide text itself lives in the SPA locale file |

When `setup_required` is true every other data/sync endpoint returns `503 {"error":{"code":"setup_required"}}`.

## Products
| GET | `/products?archived=` | any | list with linked store apps, per-platform 30-day downloads, rating |
| POST | `/products` | admin | `{name, icon_url?, description?}` |
| GET | `/products/{id}` | any | product + store apps |
| PUT | `/products/{id}` | admin | name, icon_url, description, archived |
| DELETE | `/products/{id}` | admin | only when no store apps linked |
| POST | `/products/{id}/apps` | admin | `{app_id}` link; `409 platform_taken` if the product already has that platform |
| DELETE | `/products/{id}/apps/{app_id}` | admin | unlink → store app becomes unassigned |
| GET | `/products/suggestions` | admin | unassigned store apps with `suggested_product_id` (if any) or a proposed new product name; nothing is linked until accepted |
| POST | `/products/suggestions/accept` | admin | `{app_id, product_id?}` — link to existing or create product from the app's name |

## Store apps
| GET | `/apps?store=&platform=&product_id=&unassigned=1` | any | active apps only, with last-synced, totals for last 30 days |
| GET | `/apps/{id}` | any | `404` for viewers if the app is ignored |
| PUT | `/apps/{id}` | admin | name, icon_url |
| POST | `/apps/{id}/ignore` | admin | `{reason?}` → unlinks from product if linked, sets `ignored_at`; `409` if a sync is currently processing it (retry after) |
| POST | `/apps/{id}/restore` | admin | clears ignore; app returns as unassigned |
| GET | `/apps/ignored` | admin | ignored list with reason, who, when, last data day |

All list/metric/review endpoints silently exclude ignored apps; there is no `include_ignored` flag outside `/apps/ignored`.

## Metrics
All metric endpoints accept the same scope filters: `product_id`, `platform`, `store`,
`app_id` (any combination; omitted = everything). `group` chooses the series dimension.

| GET | `/metrics/summary?from=&to=&<scope>` | any | totals + deltas vs previous period: downloads, updates, uninstalls, crashes, avg rating, review count; plus `by_platform:{ios:{…}, android:{…}}` |
| GET | `/metrics/series?from=&to=&metric=downloads&group=product\|platform\|store\|app\|country&bucket=day\|week\|month&<scope>` | any | `{buckets:[…], series:[{key,label,platform?,values:[…]}]}` |
| GET | `/metrics/countries?from=&to=&metric=&<scope>` | any | top-N breakdown |
| GET | `/metrics/products?from=&to=` | any | one row per product: totals + per-platform split (feeds the dashboard table) |
| GET | `/metrics/export.csv?…` | any | same filters, CSV download |

`metric` ∈ `downloads, redownloads, updates, uninstalls, active_devices, crashes, anrs`. Ratings are not a time series in v1; the current per-app snapshot comes with `/apps` and `/products`.

## Reviews
| GET | `/reviews?product_id=&platform=&store=&app_id=&rating=&from=&to=&q=&country=&replied=&page=&per_page=` | any | paginated, newest first; `q` uses FTS |
| GET | `/reviews/{id}` | any | |
| GET | `/reviews/stats?from=&to=&<scope>` | any | rating histogram 1–5, avg, count, per-platform split |

## Sync
| GET | `/sync/status` | any | per store: configured, running run (with progress), last run, next scheduled |
| POST | `/sync/runs` | admin | `{store, mode}` → `201 {run}` or `409` |
| GET | `/sync/runs?store=&page=` | any | history |
| GET | `/sync/runs/{id}` | any | run + progress |
| GET | `/sync/runs/{id}/logs?after=` | any | incremental log lines |
| POST | `/sync/runs/{id}/cancel` | admin | |
| POST | `/sync/reset` | admin | `{store, confirm:"<store name>"}` wipes store data & checkpoints |

## Settings (admin; `GET` also for viewer with only the `ui.*` keys)
| GET | `/settings` | |
| PUT | `/settings` | partial map `{key:value}`; validated per key |

## System
| GET | `/version` | — | `{version, commit, build_time, go}` |
| GET | `/health` | — | `{ok:true, db:"ok", setup_required:false}`; used by Docker `HEALTHCHECK` |
