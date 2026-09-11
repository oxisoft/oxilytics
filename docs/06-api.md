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

## Apps
| GET | `/apps?store=&enabled=` | any | list with last-synced, totals for last 30 days |
| GET | `/apps/{id}` | any | |
| PUT | `/apps/{id}` | admin | name, icon_url, product_key, enabled |

## Metrics
| GET | `/metrics/summary?from=&to=&store=&app_id=` | any | totals + deltas vs previous period: downloads, updates, uninstalls, crashes, avg rating, review count |
| GET | `/metrics/series?from=&to=&metric=downloads&group=app\|store\|country&app_id=&store=` | any | `{days:[…], series:[{key,label,values:[…]}]}` |
| GET | `/metrics/countries?from=&to=&metric=&app_id=` | any | top-N breakdown |
| GET | `/metrics/export.csv?…` | any | same filters, CSV download |

`metric` ∈ `downloads, redownloads, updates, uninstalls, active_devices, crashes, anrs, rating_avg, rating_count`.

## Reviews
| GET | `/reviews?store=&app_id=&rating=&from=&to=&q=&country=&replied=&page=&per_page=` | any | paginated, newest first; `q` uses FTS |
| GET | `/reviews/{id}` | any | |
| GET | `/reviews/stats?from=&to=&app_id=` | any | rating histogram 1–5, avg, count |

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
| GET | `/health` | — | `{ok:true, db:"ok"}`; used by Docker `HEALTHCHECK` |
