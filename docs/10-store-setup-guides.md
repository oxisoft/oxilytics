# 10 — Store setup guides

Source of truth for the in-app guides (`#/setup/appstore`, `#/setup/googleplay`). The
SPA renders the same steps from `web/src/locales/en.json`; keep both in sync (a CI check
compares step counts).

Both guides end with the same three actions: **put the values in `.env` / mount the
file → restart the container → press "Test connection"**.

---

## App Store Connect

**What you get:** apps, downloads / redownloads / updates / deletions per day and
territory, crashes per day, customer reviews, store-wide rating.

**You need:** an Apple Developer Program membership and an App Store Connect user
with the **Admin** or **App Manager** role (only they can create API keys and request
analytics reports).

### Steps
1. Open **App Store Connect → Users and Access → Integrations → App Store Connect API**
   (https://appstoreconnect.apple.com/access/integrations/api).
2. If you see "Request Access", request it once for the team (Account Holder must accept).
3. Under **Team Keys** click **Generate API Key** (the "+" button).
   - Name: `oxilytics`
   - Access: **App Manager** (Admin also works; *Sales and Reports* / *Developer* are **not** enough for Analytics Reports).
4. Download the key file **`AuthKey_XXXXXXXXXX.p8`** — you can do this **only once**. Store it safely.
5. Note the **Key ID** (10 chars, shown in the row) and the **Issuer ID** (UUID at the top of the page).
6. Copy the file into the `secrets/` folder next to `docker-compose.yml` and set:

   ```
   OXI_ASC_KEY_ID=ABC123DEFG
   OXI_ASC_ISSUER_ID=69a6de7e-xxxx-xxxx-xxxx-xxxxxxxxxxxx
   OXI_ASC_KEY_FILE=/secrets/AuthKey_ABC123DEFG.p8
   ```
7. `docker compose up -d` (restart), then **Test connection**.

### What "Test connection" checks
1. Key file exists and is a valid EC P-256 private key.
2. JWT accepted: `GET /v1/apps?limit=1` returns 200.
3. Analytics reports permission: `GET /v1/analyticsReportRequests?limit=1` returns 200 (403 → role too low).
4. Reviews: `GET /v1/apps/{first}/customerReviews?limit=1`.

### Common errors
| Message | Cause / fix |
|---------|-------------|
| `401 NOT_AUTHORIZED` | Key ID / Issuer ID mismatch, or key revoked. |
| `403 FORBIDDEN` on analytics | Key role is not App Manager/Admin — generate a new key. |
| `403 … API access not enabled` | Step 2 not completed. |
| No apps returned | The key belongs to another team. |

### Notes
- Historical analytics (full sync) is produced by Apple asynchronously; the first full
  run can take **several hours** waiting for the `ONE_TIME_SNAPSHOT` report — that is normal.
- Apple keeps daily analytics for a limited period; run the first full sync soon after setup.

---

## Google Play

**What you get:** installs / uninstalls / upgrades per day and country, active devices,
crashes and ANRs per day, ratings per day, reviews (full history via reports, last 7
days live).

**You need:** a Google Play Console **owner** or an admin who can manage users, and any
Google Cloud project.

### Steps
1. **Create a service account**
   1. Open https://console.cloud.google.com/iam-admin/serviceaccounts (pick or create a project, e.g. `oxilytics`).
   2. **Create service account** — name `oxilytics`, no project roles needed.
   3. Open it → **Keys → Add key → Create new key → JSON**. Save the file as `gplay-sa.json`. Note the service account e-mail (`oxilytics@<project>.iam.gserviceaccount.com`).
2. **Enable the API** — https://console.cloud.google.com/apis/library/androidpublisher.googleapis.com → **Enable** (same project).
3. **Invite the service account in Play Console**
   1. https://play.google.com/console → **Users and permissions → Invite new users**.
   2. E-mail: the service-account e-mail from 1.3.
   3. Permissions (account-level, all apps): **View app information and download bulk reports (read-only)** and **View app quality information (read-only)**. *Reply to reviews* and financial permissions are not needed.
   4. Send invite (a service account accepts automatically).
4. **Find the reports bucket**
   1. Play Console → **Download reports → Statistics** (any app).
   2. Click **Copy Cloud Storage URI** — it looks like `gs://pubsite_prod_8241096735521408897/stats/installs/`.
   3. The bucket name is the `pubsite_prod_…` part.

   > **The bucket name has no `rev_`.** Google's own help page still documents
   > `pubsite_prod_rev_…`, and older accounts do use that form, but the URI the
   > Console copies today is `pubsite_prod_<developer-id>` — the digits are the
   > Play developer account ID. Copy the real URI rather than composing one from
   > the docs; a wrong bucket fails as a 403, which reads like a permissions
   > problem and sends you debugging the wrong thing.
5. Copy `gplay-sa.json` into `secrets/` and set:

   ```
   OXI_GPLAY_SA_FILE=/secrets/gplay-sa.json
   OXI_GPLAY_BUCKET=pubsite_prod_8241096735521408897
   ```
6. `docker compose up -d` (restart), then **Test connection**.

### What "Test connection" checks
1. JSON file parses, has `client_email` and `private_key`, token obtained.
2. Bucket listable: `GET storage/v1/b/{bucket}/o?prefix=stats/installs/&maxResults=1` returns 200 and at least one object.
3. Android Publisher: `reviews.list` on the first package returns 200 (403 → permission or API not enabled).

### Common errors
| Message | Cause / fix |
|---------|-------------|
| `403` listing the bucket | Service account not invited, or invited without *download bulk reports*; propagation takes up to 24 h after inviting. |
| Bucket lists 0 objects | New account — Google generates reports the next day. |
| `403 … androidpublisher API has not been used in project` | Step 2 (enable API). |
| `401 invalid_grant` | JSON key deleted/rotated in Cloud Console. |

### Notes
- Google generates report files once a day (early morning UTC); a delta run before that sees yesterday's data.
- Reports exist from the day the account first had installs; the full sync walks every monthly file.

---

## Microsoft Store (future)
Placeholder card in the UI: "Coming later". Partner Center analytics API requires an
Azure AD app registration; the guide will follow the same structure.
