# 01 — Overview

## Goal

Give an app publisher one place to see how their apps perform across stores, without depending on
each store's console, and keep the history for as long as we want (stores keep it for a
limited time: Google Play CSVs go back to the account start, but App Store *Sales* daily
reports are only kept for ~1 year, and Google Play `reviews.list` returns only the last 7 days).

## Scope of v1

| Area | App Store Connect | Google Play |
|------|-------------------|-------------|
| App catalogue (name, id, icon, platform) | `GET /v1/apps` + iTunes lookup | package list from bucket file names + Android Publisher `edits.listings` / `edits.images` |
| Downloads / installs per day, per country | Analytics Reports API (`App Store Downloads` / `App Store Installation and Deletion`) | `stats/installs/*_overview.csv` and `*_country.csv` |
| Updates | same reports | ❌ **not reported by Play at all** |
| Uninstalls / deletions | same reports (⚠️ **weekly and volume-gated**) | same CSVs (daily) |
| Active devices | — | `stats/installs/*_overview.csv` |
| Rating: current store-wide average + count (snapshot per sync, no history) | iTunes lookup (`averageUserRating`, `userRatingCount`) | `stats/ratings/*_overview.csv` latest `Total Average Rating` |
| Reviews (text, stars, author, version, country, date) | `GET /v1/apps/{id}/customerReviews` | `reviews/reviews_*.csv` (history) + `reviews.list` (last 7 days) |
| Crashes per day | Analytics Reports API (`App Crashes`) | `stats/crashes/*_overview.csv` |

Everything is **read-only** against the stores. v1 does not reply to reviews.

### Metric coverage is asymmetric, and the UI must respect that

The two stores do not report the same things, in **both** directions: Google
publishes no updates column at all, while Apple publishes no active-device count
and gates deletions behind a weekly, volume-limited report.

**A cross-store KPI is therefore only honest when both stores actually report the
metric.** Where they do not, the dashboard omits the card rather than showing a
total that silently means one platform — a portfolio "uninstalls" figure that is
really Android-only invites exactly the wrong conclusion. Every remaining KPI
carries its per-platform split underneath so the composition stays visible.

## Non-goals (v1)

- Revenue, proceeds, subscriptions, in-app purchases.
- Rating **history** (daily average / count series). v1 stores only the current snapshot.
- Single-app sync; sync always covers a whole store.
- Microsoft Store / Partner Center.
- Multi-tenant (several companies) — one workspace only.
- Push notifications / e-mail alerts (a "review below 3★" alert is a v2 candidate).
- Editing anything on the stores.
- Mobile app; the SPA is responsive, that is enough.

## Minimum configuration

The app is useless without store data, so **at least one store must be configured**
(credentials present and valid). Start-up still succeeds — otherwise nobody could read
the in-app guide — but the process enters **setup mode**: a warning is logged, `/api/health`
reports `setup_required: true`, and the UI shows only the *Setup* screen (login still
required) until an admin has provided credentials and restarted the container. Adding
the second store later is the same procedure.

## Users

- **admin** — everything: users, settings, run sync, view data.
- **viewer** — view dashboard, apps, reviews, sync status. Cannot run sync or change settings.

The first admin is bootstrapped from env vars on first start (see 08-deployment).

## Glossary

- **Store** — `appstore` or `googleplay`.
- **Product** — what the publisher ships ("My Notes"). Owns one **store app** per platform. All dashboards are product-centric.
- **Store app** — one listing on one store (`appstore`/ios, `googleplay`/android, later `msstore`/windows). Belongs to at most one product; unlinked store apps are shown in an "Unassigned" bucket until an admin links them or accepts the suggested match.
- **Ignored store app** — a listing an admin does not want to see (stale, test build, discontinued). Ignored apps are excluded from sync, from all lists, counts and totals, and from the viewer role entirely; admins see them only in *Settings → Ignored apps*. Ignoring is reversible; existing data is kept.
- **Platform** — `ios`, `macos`, `android`, `windows`; derived from the store app, used as the breakdown dimension. An App Store listing that ships iOS and macOS under one id is `ios`; a macOS-only listing is `macos`.
- **Sync run** — one execution of the sync for one store, in mode `full` or `delta`.
- **Checkpoint** — per (store, source) marker of what has been ingested, used by delta runs.
- **Metric day** — one row per (app, date, country) with counters.
