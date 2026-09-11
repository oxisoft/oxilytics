# 07 — UI screens

Svelte 5 (runes) SPA, hash routing (`svelte-spa-router`), Tailwind 4, Chart.js for
charts, svelte-i18n with `en.json`. Dark/light follows `prefers-color-scheme` with a
manual toggle persisted in `localStorage`. No icon package: the ~15 icons needed are
inline SVG components in `src/lib/icons/`.

## Layout

```
┌──────────────────────────────────────────────────────────────┐
│ ◈ Oxilytics   Dashboard  Apps  Reviews  Sync  [Settings]  ● me│  top bar (Settings admin-only)
├──────────────────────────────────────────────────────────────┤
│  [global filter bar: store ▾ | app ▾ | date range ▾ ]          │  on data screens
│                                                                │
│  page content                                                  │
│                                                                │
├──────────────────────────────────────────────────────────────┤
│ v1.2.0 (a1b2c3d) · last sync: App Store 06:31, Google Play 06:34│  footer
└──────────────────────────────────────────────────────────────┘
```

Global filter state (store, app, date range) lives in a store and is mirrored into the
URL hash query so links are shareable. Date-range presets: 7d, 30d, 90d, 12m, YTD, custom.

Common states every data screen implements: loading skeleton, empty ("No data yet — run
a sync"), error banner with retry, and a "store not configured" notice.

## Routes

| Route | Screen | Role |
|-------|--------|------|
| `#/login` | Login | public |
| `#/login/totp` | TOTP step | public (pending session) |
| `#/` | Dashboard | any |
| `#/apps` | Apps list | any |
| `#/apps/:id` | App detail | any |
| `#/reviews` | Reviews | any |
| `#/reviews/:id` | Review detail (modal over list, also deep-linkable) | any |
| `#/sync` | Sync | any (actions admin) |
| `#/sync/runs/:id` | Sync run detail | any |
| `#/settings` | Settings – General | admin |
| `#/settings/users` | Settings – Users | admin |
| `#/settings/stores` | Settings – Stores (read-only status of credentials) | admin |
| `#/profile` | My profile | any |
| `#/about` | About (modal) | any |
| `*` | 404 | |

## 1. Login
- Email, password, "Sign in". Error inline ("Invalid credentials"), rate-limit message.
- Redirects to the route the user originally requested.

## 2. TOTP step
- 6-digit input, autofocus, auto-submit on 6 chars; link "Use a recovery code".

## 3. Dashboard (`#/`)
Purpose: one glance at how everything is doing.
- **KPI cards** (for filter range, with delta vs previous period, colour-coded):
  Downloads · Updates · Uninstalls · Crashes · Avg rating · New reviews.
- **Downloads over time** — line chart, one series per store (or per app when a store is selected). Toggle: daily / weekly / monthly buckets.
- **Crashes over time** — bar chart, same grouping.
- **Ratings** — small line chart of `rating_total_avg` per app + histogram of review stars in range.
- **Top countries** — horizontal bar, top 10 by downloads, with share %.
- **Apps table** — one row per app: icon, name, store badge, downloads (range), Δ%, crashes, rating, last review date; click → App detail. Sortable.
- **Recent reviews** — last 5, star + first line, click → Reviews.
- **Sync banner** — if a sync is running: progress bar; if last run failed: warning link to Sync.

## 4. Apps (`#/apps`)
- Table of all apps grouped by `product_key` when set (one row expands into iOS + Android).
- Columns: icon, name, store, id/package, platform, enabled, first seen, last synced, 30-day downloads.
- Filter store / enabled; search.
- Admin: inline toggle **enabled**, edit dialog (name, icon URL, product key).
- Empty state: "No apps yet. Run a full sync to discover apps."

## 5. App detail (`#/apps/:id`)
- Header: icon, name, store badge, bundle/package, link to store page, "Sync this app" (admin, delta, single app).
- Same KPI cards as dashboard, scoped.
- Tabs:
  - **Downloads** — line chart downloads / redownloads / updates / uninstalls (toggle series); table by day (paginated) with CSV export.
  - **Countries** — map-free: table + bar chart of downloads by country, range-scoped.
  - **Crashes** — bar chart crashes (+ ANRs on Android); crash rate per 1k downloads line.
  - **Ratings** — `rating_total_avg` over time, daily rating count, histogram.
  - **Reviews** — the Reviews list pre-filtered to this app.
- Sibling link: "See Android version →" when `product_key` matches.

## 6. Reviews (`#/reviews`)
- Filter bar: store, app, rating (1–5 multi), country, date range, replied yes/no, full-text search.
- Summary strip: count, average, histogram bars (click a bar to filter).
- List (infinite scroll or paged 50): star rating, title, body (clamped to 3 lines, expand), author, app + version, country flag-as-text code, date, "replied" chip.
- Row click → **Review detail** modal: full text, developer reply, metadata, link to store.
- CSV export of current filter.

## 7. Sync (`#/sync`)
- Two **store cards** (App Store, Google Play):
  - configured? (green/grey), last run (status, when, duration, rows), next scheduled run.
  - running: progress bar `apps_done/apps_total`, current step text, elapsed, **Cancel** (admin).
  - buttons (admin): **Sync now (delta)**, **Full sync…** (confirm dialog explaining duration and that it re-fetches everything), **Reset data…** (type store name to confirm).
- **Schedule** summary: "Daily at 06:30 Europe/Warsaw · App Store, Google Play" with link to settings.
- **History** table: started, store, mode, trigger (user name / schedule), status, duration, metrics rows, reviews rows, error snippet; click → run detail.

## 8. Sync run detail (`#/sync/runs/:id`)
- Header with status badge, mode, trigger, range, timing, counters, error.
- Live-updating log view (polls `/logs?after=`) with level filter; auto-scroll toggle.
- Per-app breakdown table from `stats` (rows, errors, duration).

## 9. Settings – General (`#/settings`)
- **Automatic sync**: enabled toggle, time picker (HH:MM), stores checkboxes, shows timezone (from env, read-only) and "next run at".
- **Delta overlap days** (1–14).
- **Retention** (days, 0 = forever).
- **Dashboard default range**.
- **Appearance** (theme) — per browser, not a server setting.
- Save button, inline validation, toast on success.

## 10. Settings – Users (`#/settings/users`)
- Table: name, email, role, TOTP on/off, disabled, last login.
- Add user dialog (email, name, role, temporary password).
- Row actions: edit role/name, reset password, disable/enable, delete (guards: not self, not last admin).

## 11. Settings – Stores (`#/settings/stores`)
Read-only, because credentials are files/env:
- Per store: configured ✔/✖, which env vars are set (values masked), key id / issuer id / service-account e-mail / bucket name, "Test connection" button (admin) → calls the store's cheapest endpoint and reports OK / error text.
- Help text: where to obtain the key, which Play Console permissions to grant.

## 12. My profile (`#/profile`)
- Name, email (read-only), change password.
- Two-factor: status; **Enable** → shows QR (rendered client-side from the otpauth URL with a small inline QR generator, no dependency) + manual secret → enter code → show recovery codes once (copy / download). **Disable** asks for password.

## 13. About (modal)
- Version, commit, build time, Go version, link to repo, licence.

## Components (`src/lib/components`)
`TopBar`, `FilterBar`, `DateRangePicker`, `KpiCard`, `LineChart`, `BarChart`,
`Histogram`, `DataTable` (sortable, paged), `StoreBadge`, `Stars`, `ProgressBar`,
`Modal`, `ConfirmDialog`, `Toast`, `EmptyState`, `Skeleton`, `LogView`, `Qr`.

## Accessibility & responsiveness
- Keyboard-navigable tables and dialogs, focus trap in modals, `aria-live` for toasts and sync progress.
- Below 768 px the top bar collapses to a menu, KPI cards stack, tables scroll horizontally.
