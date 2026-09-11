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
| GET | `/products/suggestions` | admin | unassigned store apps with `suggested_product_id` or a proposed new product name |
| POST | `/products/suggestions/accept` | admin | `{app_id, product_id?}` — link to existing or create product from the app's name |

## Store apps
| GET | `/apps?store=&platform=&product_id=&unassigned=1` | any | list with last-synced, totals for last 30 days |
| GET | `/apps/{id}` | any | |
| PUT | `/apps/{id}` | admin | name, icon_url, enabled |

## Metrics
All metric endpoints accept the same scope filters: `product_id`, `platform`, `store`,
`app_id` (any combination; omitted = everything). `group` chooses the series dimension.

| GET | `/metrics/summary?from=&to=&<scope>` | any | totals + deltas vs previous period: downloads, updates, uninstalls, crashes, avg rating, review count; plus `by_platform:{ios:{…}, android:{…}}` |
| GET | `/metrics/series?from=&to=&metric=downloads&group=product\|platform\|store\|app\|country&bucket=day\|week\|month&<scope>` | any | `{buckets:[…], series:[{key,label,platform?,values:[…]}]}` |
| GET | `/metrics/countries?from=&to=&metric=&<scope>` | any | top-N breakdown |
| GET | `/metrics/products?from=&to=` | any | one row per product: totals + per-platform split (feeds the dashboard table) |
| GET | `/metrics/export.csv?…` | any | same filters, CSV download |

`metric` ∈ `downloads, redownloads, updates, uninstalls, active_devices, crashes, anrs, rating_avg, rating_count`.

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
