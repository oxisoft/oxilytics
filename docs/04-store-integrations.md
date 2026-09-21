# 04 — Store integrations

Both clients live in `internal/storeclient/<store>` and expose typed methods. They do
retries with exponential backoff on `429`/`5xx`, honour `Retry-After`, and return
structured errors (`ErrAuth`, `ErrRateLimited`, `ErrNotReady`, `ErrNotFound`).
All HTTP is recorded in tests as fixtures under `testdata/`.

## App Store Connect

### Credentials
- Team API key (`Keys` in App Store Connect → Users and Access → Integrations), role
  **Sales and Reports** is not enough for analytics; use **App Manager** or **Admin**.
- Files: `OXI_ASC_KEY_FILE` (`.p8`), plus `OXI_ASC_KEY_ID`, `OXI_ASC_ISSUER_ID`.
- Auth: ES256 JWT, `aud=appstoreconnect-v1`, lifetime ≤ 20 min; the client caches a
  token and renews at 15 min.

### Endpoints used
| Purpose | Endpoint |
|---------|----------|
| App catalogue | `GET /v1/apps?fields[apps]=name,bundleId,sku,primaryLocale&limit=200` |
| App icon | `GET /v1/apps/{id}/appInfos` → `appStoreVersions` icon is awkward; simpler: `https://itunes.apple.com/lookup?id={id}` → `artworkUrl512`, `averageUserRating`, `userRatingCount` (public, no auth, per `country=` param) |
| Reviews | `GET /v1/apps/{id}/customerReviews?sort=-createdDate&limit=200&include=response` — pages with `links.next`; fields: rating, title, body, reviewerNickname, createdDate, territory |
| Analytics reports (downloads, installs/deletions, crashes) | Analytics Reports API, see below |

### Analytics Reports API (downloads, deletions, crashes)
This is the only Apple API that gives per-day crash counts and full download history.

1. `POST /v1/analyticsReportRequests` with `accessType`:
   - `ONE_TIME_SNAPSHOT` — historical data (used by **full** sync). Apple generates it
     asynchronously; can take hours. The run polls until instances exist.
   - `ONGOING` — daily incremental (used by **delta**). Created once per app, id stored in
     `sync_checkpoints.cursor` for source `asc_report_request`.
2. `GET /v1/analyticsReportRequests/{id}/reports?filter[name]=…` — report names used:
   - `App Store Downloads` (category `APP_USAGE`) → `downloads`, `redownloads`, `updates` by territory
   - `App Store Installation and Deletion` → `uninstalls`
   - `App Crashes` (category `APP_USAGE` / `PERFORMANCE`) → `crashes`
3. `GET /v1/analyticsReports/{id}/instances?filter[granularity]=DAILY` → instances per day
4. `GET /v1/analyticsReportInstances/{id}/segments` → signed `url`s to gzipped CSV; download, parse, upsert.

Quirks:
- Data lags 1–3 days; delta always re-fetches `sync.delta.overlap_days`.
- Segment CSVs are per processing date, not calendar date; the `Date` column is authoritative.
- Territory codes are Apple's (mostly ISO-2; map the few exceptions).
- Rate limit is per key, roughly 3 600 requests/hour; the client throttles to 1 req/s.

### Ratings
Apple has no historical rating endpoint and v1 needs none. Once per run the iTunes
lookup (`country=us`) fills `apps.rating_avg` / `rating_count` — a snapshot, overwritten each time.

### Icons, and why an App Store app can legitimately have none

The same public iTunes lookup supplies `artworkUrl512`. Two properties of it
matter, and both look like bugs when you hit them:

1. **It only covers *published* apps.** An app in review, unreleased, or pulled
   returns `resultCount: 0`. There is nothing to fetch, and no permission that
   changes this.
2. **It is storefront-scoped.** `country=` is currently **hardcoded to `us`**, so
   an app not available in the US storefront returns nothing even when published
   elsewhere. *(Known limitation — the country should follow the app's actual
   availability or sweep storefronts.)*

Verified by sweeping 16 storefronts: two of our own apps are absent from **every**
one (not public anywhere), while a published app returns in all 16. So a missing
Apple icon is usually the listing's status, not a broken integration.

Products that also ship on Android fall back to the Play listing icon, which is why
the dashboard can still show an icon for an app with no public App Store presence.

## Google Play

### Credentials
- Service account JSON (`OXI_GPLAY_SA_FILE`). In Play Console → Users and permissions,
  invite the service account e-mail with **View app information and download bulk reports**
  and **View financial data** is *not* needed. Also grant **Reply to reviews** = no; **View reviews** yes.
- Bucket name from Play Console → Download reports → *Copy Cloud Storage URI*: `pubsite_prod_rev_…`.
- Scopes: `https://www.googleapis.com/auth/devstorage.read_only` and `https://www.googleapis.com/auth/androidpublisher`.

### Bucket files (`storage.googleapis.com/storage/v1/b/{bucket}/o?prefix=`)
| Prefix | File | Columns used |
|--------|------|--------------|
| `stats/installs/` | `installs_{pkg}_{YYYYMM}_overview.csv` | Date, Daily Device Installs, Daily Device Uninstalls, Daily Device Upgrades, Daily User Installs, Daily User Uninstalls, Active Device Installs |
| `stats/installs/` | `installs_{pkg}_{YYYYMM}_country.csv` | same + Country |
| `stats/ratings/` | `ratings_{pkg}_{YYYYMM}_overview.csv` | only the last row's `Total Average Rating` → `apps.rating_avg` (snapshot; no history in v1) |
| `stats/crashes/` | `crashes_{pkg}_{YYYYMM}_overview.csv` | Date, Daily Crashes, Daily ANRs |
| `reviews/` | `reviews_{pkg}_{YYYYMM}.csv` | Package Name, App Version Code/Name, Reviewer Language, Device, Review Submit Date and Time, Star Rating, Review Title, Review Text, Developer Reply Date and Time, Developer Reply Text, Review Link |

Quirks:
- Files are **UTF-16 LE with BOM**, comma-separated; the client transcodes.
- The current month's file is rewritten daily; Google also restates the last few days. Delta re-downloads the current and previous month regardless of `generation`, older months only if `generation` changed (`ingested_objects`).
- Google exposes no total ratings count through reports or API → `apps.rating_count` stays NULL on Android; the UI shows the average only.
- Package list = distinct `{pkg}` in file names. Display name and icon are read through the Android Publisher API: `edits.insert` → `edits.listings.get(defaultLanguage)` → `title`, then the icon, then `edits.delete`. Clumsy but official and cheap (once per full sync). If it fails, the store app is created with the package name as name and the admin can override it in the Apps screen.

  **The icon URL is:**

  ```
  GET …/applications/{pkg}/edits/{editId}/listings/{lang}/icon
  ```

  > ⚠️ **Not `…/listings/{lang}/phone/icon`.** Screenshots nest under a form
  > factor (`phone`, `tenInchTablet`, …); the icon does **not**, and the form-factor
  > path returns **404** for every app. This cost a long debugging session because
  > the 404 was swallowed by an `err == nil` check — every Play app silently stored
  > a NULL icon and nothing was logged — *and* the unit-test mock served the same
  > wrong path as the client, so the suite was green while production failed. The
  > mock now serves the real path and a regression test asserts the 404 is
  > **reported, not swallowed** (`Listing.IconErr`).

  A throwaway edit (`POST …/edits`) is required first and works with a read-only
  service account. Delta runs retry the icon lookup whenever an app still has none,
  so icon-less rows backfill instead of waiting for the next full sync.

### Reviews API
`GET https://androidpublisher.googleapis.com/androidpublisher/v3/applications/{pkg}/reviews?maxResults=100&translationLanguage=en`
returns only reviews **from the last 7 days** with pagination `token`. Delta sync uses
it to get today's reviews immediately; the monthly CSV fills history and edits.

## Mapping to `metric_days`

| metric | Apple | Google |
|--------|-------|--------|
| downloads | `App Store Downloads`.`First-Time Downloads` | `Daily User Installs` |
| redownloads | `Redownloads` | — |
| updates | `Updates` | `Daily Device Upgrades` |
| uninstalls | `Installation and Deletion`.`Deletions` | `Daily User Uninstalls` |
| active_devices | — | `Active Device Installs` |
| crashes | `App Crashes`.`Crashes` | `Daily Crashes` |
| anrs | — | `Daily ANRs` |
| apps.rating_avg / rating_count (snapshot) | iTunes lookup | latest `Total Average Rating` / NULL |
