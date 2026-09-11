# Oxilytics

Self-hosted analytics for app publishers. One Go binary, one SQLite file, one
Docker image. Pulls downloads, updates, uninstalls, crashes, ratings and reviews
from **App Store Connect** and **Google Play** into a single dashboard, grouped
per *product* (your app across iOS + Android, Windows later).

- No SaaS, no telemetry, no tracking SDK in your apps — reads only the store APIs.
- Full sync (whole history) and delta sync (since last run), manual or daily.
- Products link the iOS and Android listings so you see one app, split by platform.
- Reviews with full-text search, filters, developer-reply status, CSV export.
- Admin / viewer roles, optional TOTP, encrypted cookie sessions.
- Store listings you don't care about can be ignored (hidden everywhere).
- Ships as a distroless multi-arch image (amd64 + arm64).

## Quick start (Docker)

```sh
mkdir oxilytics && cd oxilytics
curl -fsSLO https://raw.githubusercontent.com/oxisoft/oxilytics/main/deploy/docker-compose.yml
curl -fsSL  https://raw.githubusercontent.com/oxisoft/oxilytics/main/deploy/.env.example -o .env
openssl rand -hex 32          # → paste as OXI_SESSION_KEY in .env
mkdir -p data secrets
sudo chown -R 65532:65532 data   # container runs as distroless "nonroot"
docker compose up -d
```

Open `http://127.0.0.1:8080`, sign in with the bootstrap admin from `.env`,
and the app guides you through connecting a store. Put the
App Store `.p8` key and/or Google service-account JSON into `./secrets` and set
the matching `OXI_*` variables — the in-app **Setup** screen tells you exactly
which, and has a *Test connection* button per store.

Run it behind a TLS-terminating reverse proxy (Caddy example in `deploy/`) and
set `OXI_BASE_URL` to the public URL.

## Configuration

Everything is environment variables (see `deploy/.env.example`). Required:

| Variable | Purpose |
|---|---|
| `OXI_SESSION_KEY` | 64 hex chars, cookie encryption key |
| `OXI_BASE_URL` | public URL, used for cookie security and CSRF |
| `OXI_BOOTSTRAP_ADMIN_EMAIL` / `_PASSWORD` | first admin (only used when the users table is empty) |

Stores (at least one):

| App Store Connect | Google Play |
|---|---|
| `OXI_ASC_KEY_ID`, `OXI_ASC_ISSUER_ID`, `OXI_ASC_KEY_FILE` | `OXI_GPLAY_SA_FILE`, `OXI_GPLAY_BUCKET` |

Optional: `OXI_ADDR` (default `:8080`), `OXI_DB_PATH` (`/data/oxilytics.db`),
`OXI_TZ` (schedule timezone, default UTC), `OXI_BACKUP_DIR`, `OXI_LOG_LEVEL`.

Runtime settings (daily sync time, which stores, retention of sync logs,
product-linking suggestions) live in **Settings** inside the app.

## Development

```sh
make dev-secrets   # throwaway store credentials so setup mode is off
make seed          # demo products / metrics / reviews into ./dev.db
make run           # API on :8080, uses ./dev.db
cd web && OXI_API=http://127.0.0.1:8080 npm run dev   # SPA on :5173 with HMR
```

`make test` runs `go vet`, `go test -race ./...` and `npm run build`.
`make build` produces `bin/oxilytics` with the SPA embedded.

Layout, data model, sync algorithm, API and screens are documented in
[`docs/`](docs/README.md).

## License

MIT — see [LICENSE](LICENSE).
