# 07 — UI screens

Svelte 5 (runes) SPA, hash routing (`svelte-spa-router`), Tailwind 4, Chart.js for
charts, svelte-i18n with `en.json`. Dark/light follows `prefers-color-scheme` with a
manual toggle persisted in `localStorage`. No icon package: the ~20 icons needed
(incl. platform glyphs for iOS / Android / Windows) are inline SVG components in `src/lib/icons/`.

The UI is **product-centric**: users think in "My Notes", not in "bundle id on
App Store". Platform is a breakdown dimension everywhere, never the primary navigation.

## Layout

```
┌────────────────────────────────────────────────────────────────────┐
│ ◈ OxiLytics   Dashboard  Products  Reviews  Sync  [Settings]   ● me │  top bar (Settings admin-only)
├────────────────────────────────────────────────────────────────────┤
│  [filter bar:  product ▾ | platform: All ▢iOS ▢Android | range ▾ ] │  on data screens
│                                                                     │
│  page content                                                       │
│                                                                     │
├────────────────────────────────────────────────────────────────────┤
│ v1.2.0 (a1b2c3d) · last sync: App Store 06:31 · Google Play 06:34   │  footer
└────────────────────────────────────────────────────────────────────┘
```

Global filter state (product, platforms, date range) lives in a store and is mirrored
into the URL hash query so links are shareable. Platform filter is a multi-toggle
(All / iOS / Android / … — only platforms with a configured store are shown). Date-range
presets: 7d, 30d, 90d, 12m, YTD, custom.

Common states every data screen implements: loading skeleton, empty ("No data yet — run
a sync"), error banner with retry, and "store not configured" notices.

## Routes

| Route | Screen | Role |
|-------|--------|------|
| `#/login` | Login | public |
| `#/login/totp` | TOTP step | public (pending session) |
| `#/setup` | Setup (store guides + status) | any; **the only screen in setup mode** |
| `#/setup/appstore`, `#/setup/googleplay` | Store guide | any |
| `#/` | Dashboard | any |
| `#/products` | Products | any |
| `#/products/:slug` | Product detail | any |
| `#/products/:slug/:platform` | Product detail scoped to one platform (same screen, filter preset) | any |
| `#/apps` | Store apps (raw listings, linking) | any (edit admin) |
| `#/reviews` | Reviews | any |
| `#/reviews/:id` | Review detail (modal over list, deep-linkable) | any |
| `#/sync` | Sync | any (actions admin) |
| `#/sync/runs/:id` | Sync run detail | any |
| `#/settings` | Settings – General | admin |
| `#/settings/users` | Settings – Users | admin |
| `#/settings/stores` | Settings – Stores (status + guides, same component as Setup) | admin |
| `#/settings/ignored` | redirects to `#/products?tab=ignored` (kept for old links) | admin |
| `#/profile` | My profile | any |
| `#/about` | About (modal) | any |
| `*` | 404 | |

## 1. Login
- Email, password, "Sign in". Error inline ("Invalid credentials"), rate-limit message.
- Redirects to the route the user originally requested (or `#/setup` in setup mode).

## 2. TOTP step
- 6-digit input, autofocus, auto-submit on 6 chars; link "Use a recovery code".

## 3. Setup (`#/setup`)
Shown as the landing page when **no store is configured**; also reachable any time from
Settings → Stores. Purpose: get an admin from "fresh container" to "first sync" without
reading external docs.

- Banner: "OxiLytics needs access to at least one store. Configure App Store Connect or
  Google Play below, restart the container, then run a full sync."
- Two **store cards**, each with: status (Not configured / Configured ✔ / Error ✖ with
  the failing check), the list of checks from `/setup/status` (env var set → file found →
  key parses → …), a **"Open guide"** button, and (admin, when configured) **"Test connection"**
  which shows step-by-step results (auth OK → apps listed: 3 → reviews readable ✔).
- Footer: "Where do these values go?" → shows the exact `docker-compose.yml` / `.env`
  snippet with the current variable names, copy button.
- Viewer role sees the same page read-only with "Ask an administrator" hint.

## 4. Store guide (`#/setup/appstore`, `#/setup/googleplay`)
Long-form, numbered, with screenshots-free text (kept in `en.json`, source of truth is
`docs/10-store-setup-guides.md`). Each step has a "done" checkbox persisted in
`localStorage` so an admin can resume. Ends with the env-var table (name, what to put,
example), a copyable `.env` block pre-filled with placeholders, and the **Test connection**
button. See 10-store-setup-guides.md for the content.

## 5. Dashboard (`#/`)
Purpose: one glance at how every product is doing, and how platforms compare.
- **KPI cards** (for filter range, delta vs previous period): Downloads · Updates ·
  Uninstalls · Crashes · Avg rating · New reviews. Each card shows a small
  **per-platform split** underneath (e.g. "iOS 1 240 · Android 3 870").
- **Downloads over time** — line chart. Series = products (when "All products") or
  platforms (when one product selected). Bucket toggle day / week / month. Legend
  click toggles series.
- **Platform share** — donut of downloads by platform for the range.
- **Crashes over time** — stacked bar by platform.
- **Ratings** — current rating per product/platform (big number + stars) + star histogram of reviews in range. No rating-over-time chart in v1.
- **Top countries** — horizontal bar, top 10 by downloads.
- **Products table** — one row per product: icon, name, platform glyphs (dim = missing
  listing), downloads (range) with per-platform mini-split, Δ%, crashes, rating, last
  review date. Click → Product detail. Sortable. Unassigned store apps count shown as a
  warning chip linking to Store apps.
- **Recent reviews** — last 5 across products, star + first line + platform glyph.
- **Sync banner** — running sync progress; last run failed → warning link to Sync.

## 6. Products (`#/products`)
- Card grid or table (toggle): icon, name, platform glyphs with per-platform 30-day
  downloads and rating, total, last synced.
- Admin: **New product**, edit (name, icon, description), archive.
- **Unassigned store apps** section (admin): each with the suggestion
  ("Looks like *My Notes* → Link" / "Create product 'My Notes'"), a
  product picker, and an **Ignore** action for listings that should not become a product. Nothing is linked without a click here — this is the one place the
  iOS ↔ Android linking happens.
- Empty state: "No products yet. Run a full sync to discover store apps, then link them here."

## 7. Product detail (`#/products/:slug`)
- Header: icon, name, **platform chips** (iOS · Android · Windows-greyed "not yet") — each
  chip links to the store listing and toggles that platform in the filter; current
  rating per platform; description; admin edit / manage links.
- KPI cards, scoped to the product, each with per-platform split.
- Tabs (every chart in every tab supports "stacked by platform" / "overlaid by platform" /
  "total"):
  - **Overview** — downloads line (per platform), platform share donut, crash rate per
    1k downloads per platform.
  - **Downloads** — downloads / redownloads / updates / uninstalls (series toggle) per
    platform; table by day with platform columns; CSV export.
  - **Countries** — table + bar of downloads by country, columns per platform.
  - **Crashes** — crashes (+ ANRs on Android) per platform; crash rate line.
  - **Ratings** — current rating & count per platform, review-star histogram per platform side by side (from reviews in range).
  - **Reviews** — Reviews list pre-filtered to this product, platform glyph per row.
  - **Store apps** (admin) — the linked listings with ids, unlink, ignore, and
    "Link another platform" picker for unassigned store apps.

## 8. Store apps (`#/apps`)
Raw view of what the stores expose, mainly for admins.
- Table: icon, name, store, platform, store id / package, product (link or
  "— unassigned —" with suggestion), first seen, last synced, 30-day downloads.
  Ignored apps never appear here.
- Filters: store, platform, unassigned only; search.
- Admin row actions: edit (name, icon URL), **Link to product** picker,
  **Create product from this app**, **Ignore…** (dialog: optional reason; warns
  "This will unlink it from *My Notes*" when linked). Bulk select → Ignore for cleaning
  up after a first full sync that discovers a dozen old listings.

## 9. Reviews (`#/reviews`)
- Filter bar: product, platform, rating (1–5 multi), country, date range, replied, full-text search.
- Summary strip: count, average, histogram (click a bar to filter), per-platform average.
- List (paged 50): platform glyph, star rating, title, body (clamped to 3 lines, expand),
  author, product + version, country, date, "replied" chip.
- Row click → **Review detail** modal: full text, developer reply, metadata, link to store.
- CSV export of current filter.

## 10. Sync (`#/sync`)
- Two **store cards** (App Store, Google Play):
  - Not configured → grey card with "Set up App Store Connect →" (to guide).
  - Configured: last run (status, when, duration, rows), next scheduled run.
  - Running: progress bar `apps_done/apps_total`, current step, elapsed, **Cancel** (admin).
  - Buttons (admin): **Sync now (delta)**, **Full sync…** (confirm dialog), **Reset data…**.
- **Schedule** summary with link to settings.
- **History** table: started, store, mode, trigger, status, duration, rows, error; click → run detail.
- After a run that discovered unassigned store apps: notice "3 new store apps need a product → Products".

## 11. Sync run detail (`#/sync/runs/:id`)
- Header with status badge, mode, trigger, range, timing, counters, error.
- Live-updating log view (polls `/logs?after=`) with level filter; auto-scroll toggle.
- Per-store-app breakdown table from `stats` (rows, errors, duration).

## 12. Settings – General (`#/settings`)
- **Automatic sync**: enabled, time (HH:MM), stores checkboxes (only configured ones
  selectable), timezone (read-only), "next run at".
- **Delta overlap days**, **Retention**, **Dashboard default range**.
- **Products**: suggest a product for new store apps by name (toggle; linking itself is always manual).
- **Appearance** — per browser.

## 13. Settings – Users (`#/settings/users`)
- Table: name, email, role, TOTP, disabled, last login. Add / edit / reset password /
  disable / delete with guards (not self, not last admin).

## 14. Settings – Stores (`#/settings/stores`)
Same component as **Setup** (store cards, checks, guides, test connection), embedded in
the settings layout. This is where an admin adds the second store later.

## 14b. Ignored apps (`#/products?tab=ignored`)
The third tab of Products, next to **Products** and **Unassigned**, so the whole
lifecycle of a listing (unassigned → linked, or unassigned → ignored → restored) is on
one screen. `#/settings/ignored` redirects here for old links.
- Table: icon, name, store, platform, store id / package, reason, ignored by, ignored
  on, last data day, rows of data kept.
- Row actions: **Restore** (returns to the Unassigned tab; next sync resumes it),
  edit reason.
- Empty state: "Nothing ignored. Use *Ignore* on an unassigned app to hide listings you don't care about."
- Viewers never see the tabs at all, so they get no hint that ignored apps exist.

## 15. My profile (`#/profile`)
- Name, email (read-only), change password, TOTP enable (QR via inline generator) /
  disable, recovery codes.

## 16. About (modal)
- Version, commit, build time, Go version, link to repo, licence.

## Components (`src/lib/components`)
`TopBar`, `FilterBar`, `PlatformToggle`, `PlatformGlyph`, `DateRangePicker`, `KpiCard`
(with split), `LineChart`, `BarChart`, `Donut`, `Histogram`, `DataTable`, `StoreBadge`,
`Stars`, `ProgressBar`, `Modal`, `ConfirmDialog`, `Toast`, `EmptyState`, `Skeleton`,
`LogView`, `Qr`, `StoreCard`, `CheckList`, `GuideStep`, `CopyBlock`, `ProductPicker`.

## Accessibility & responsiveness
- Keyboard-navigable tables and dialogs, focus trap in modals, `aria-live` for toasts and sync progress.
- Below 768 px the top bar collapses, KPI cards stack, tables scroll horizontally.
