# 08 — Build, deployment, operations

## Dockerfile (multi-stage)

```dockerfile
# 1. frontend
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build                      # → /src/web/dist

# 2. backend
FROM golang:1.25-alpine AS build
ARG VERSION=dev COMMIT=none BUILD_TIME=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X oxilytics/internal/version.Version=$VERSION -X oxilytics/internal/version.Commit=$COMMIT -X oxilytics/internal/version.BuildTime=$BUILD_TIME" \
    -o /oxilytics ./cmd/server

# 3. runtime
FROM gcr.io/distroless/static:nonroot
COPY --from=build /oxilytics /oxilytics
USER nonroot
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/oxilytics"]
```

`web/dist/.gitkeep` is committed so `go:embed` compiles without a frontend build (dev
mode serves Vite on :5173 with `/api` proxied to :8080).

## docker-compose (`deploy/docker-compose.yml`)

```yaml
services:
  oxilytics:
    image: ghcr.io/oxisoft/oxilytics:1.0.0
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"          # only the reverse proxy reaches it
    env_file: .env
    environment:
      OXI_ADDR: 0.0.0.0:8080           # inside the container; host binding above is localhost
      OXI_DB_PATH: /data/oxilytics.db
      OXI_BACKUP_DIR: /data/backups
      OXI_ASC_KEY_FILE: /secrets/AuthKey.p8
      OXI_GPLAY_SA_FILE: /secrets/gplay-sa.json
    volumes:
      - ./data:/data
      - ./secrets:/secrets:ro
    healthcheck:
      test: ["CMD", "/oxilytics", "-healthcheck"]   # distroless has no curl; binary self-checks
      interval: 30s
      timeout: 5s
      retries: 3
```

`.env.example` lists every `OXI_*` var. `secrets/` and `data/` are git-ignored.

Reverse proxy: `deploy/Caddyfile.example`:
```
analytics.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## First start

1. `cp .env.example .env`, set `OXI_SESSION_KEY` (`openssl rand -hex 32`), bootstrap admin, and credentials for **at least one store** (see 10-store-setup-guides.md — the same text is inside the app).
2. Put `AuthKey.p8` and/or `gplay-sa.json` in `secrets/`.
3. `docker compose up -d` → migrations run, admin created, UI at the proxy host.
4. Log in. If no store credentials were provided you land on **Setup** with the guides; follow one, restart, **Test connection**.
5. Sync → **Full sync** for each configured store.
6. Products → link the discovered iOS/Android store apps into products (or accept the suggestions).

## Backups

- Cron `03:15` local: `VACUUM INTO '/data/backups/oxilytics-YYYYMMDD.db'`, then keep the newest `OXI_BACKUP_KEEP`.
- The backup file is a consistent single-file copy; restore = stop container, replace `oxilytics.db`, delete `-wal`/`-shm`, start.
- Off-site copy is the host's job (restic/rclone of `./data/backups`).

> ⚠️ **Never copy a live `oxilytics.db` with `cp`/`scp`.** With WAL enabled you
> capture a torn file that fails `PRAGMA integrity_check` ("database disk image is
> malformed"). Use the backup file above, or `sqlite3 db ".backup 'out.db'"`.
>
> One further trap when moving a copy to another machine: `.backup` writes in the
> **dumping** SQLite's format, so a file produced by a newer SQLite (e.g. 3.53 in a
> container) can be rejected as malformed by an older local binary (3.50) even
> though the bytes transferred are byte-identical. When the two ends may differ,
> move a **SQL text dump** (`sqlite3 db .dump > out.sql`) and restore it — that is
> version-independent.

## Upgrades

Pull new tag, `docker compose up -d`. Migrations are forward-only; a failed migration
aborts start-up with the error in logs and the DB untouched (goose runs each migration in a transaction).

## CI (`.github/workflows/ci.yml`)

- **On every push / PR**: `go vet ./...`, `go test -race ./...`, `npm ci && npm run build`, `go build`.
- **On tag `v*`**: `docker buildx build --platform linux/amd64,linux/arm64` with `VERSION/COMMIT/BUILD_TIME` build-args, push `ghcr.io/oxisoft/oxilytics:{tag}` and `:latest`; create GitHub release with the changelog section.
- Go and npm caches enabled; Dependabot for gomod, npm, actions (weekly).

## Local development

```
make dev        # runs `go run ./cmd/server` with OXI_DB_PATH=./dev.db and `npm run dev` in web/ (Vite proxies /api)
make test       # go test ./... (DB tests create a temp SQLite file and apply migrations)
make build      # frontend + backend into ./bin/oxilytics
make image      # docker build with version args
```
