VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     = -s -w \
  -X github.com/oxisoft/oxilytics/internal/version.Version=$(VERSION) \
  -X github.com/oxisoft/oxilytics/internal/version.Commit=$(COMMIT) \
  -X github.com/oxisoft/oxilytics/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: dev dev-secrets seed test build web image lint

## dev: run the API on :8080 with a local dev.db; run `npm run dev` in web/ separately (Vite proxies /api)
dev:
	OXI_DB_PATH=./dev.db OXI_SESSION_KEY=$${OXI_SESSION_KEY:-0000000000000000000000000000000000000000000000000000000000000000} \
	OXI_BOOTSTRAP_ADMIN_EMAIL=$${OXI_BOOTSTRAP_ADMIN_EMAIL:-admin@local} OXI_BOOTSTRAP_ADMIN_PASSWORD=$${OXI_BOOTSTRAP_ADMIN_PASSWORD:-adminadmin1} \
	OXI_BACKUP_DIR=./dev-backups OXI_LOG_LEVEL=debug \
	OXI_ASC_KEY_ID=DEVKEY OXI_ASC_ISSUER_ID=00000000-0000-0000-0000-000000000000 OXI_ASC_KEY_FILE=./dev-secrets/AuthKey.p8 \
	OXI_GPLAY_SA_FILE=./dev-secrets/gplay-sa.json OXI_GPLAY_BUCKET=pubsite_prod_rev_dev \
	go run ./cmd/server

## dev-secrets: throwaway store credentials so the dev server leaves setup mode
dev-secrets:
	go run ./cmd/devsecrets -dir ./dev-secrets

## seed: demo products / metrics / reviews into ./dev.db
seed:
	go run ./cmd/seed -db ./dev.db

test:
	go vet ./...
	go test -race ./...

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/oxilytics ./cmd/server

image:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) -t oxilytics:$(VERSION) .
