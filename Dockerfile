# syntax=docker/dockerfile:1
# Build stages run natively on the build host ($BUILDPLATFORM); Go cross-compiles
# for $TARGETOS/$TARGETARCH. Only the final scratch-like stage is per-platform,
# so multi-arch builds never need QEMU emulation for node/go.

# 1. frontend
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. backend
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X github.com/oxisoft/oxilytics/internal/version.Version=${VERSION} -X github.com/oxisoft/oxilytics/internal/version.Commit=${COMMIT} -X github.com/oxisoft/oxilytics/internal/version.BuildTime=${BUILD_TIME}" \
    -o /oxilytics ./cmd/server
# /data must be writable by nonroot (uid 65532); named volumes inherit these perms
RUN mkdir -p /data /data/backups && chown -R 65532:65532 /data

# 3. runtime
FROM gcr.io/distroless/static:nonroot
COPY --from=build /oxilytics /oxilytics
COPY --from=build --chown=65532:65532 /data /data
USER nonroot
VOLUME ["/data"]
EXPOSE 8080
ENV OXI_ADDR=0.0.0.0:8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/oxilytics", "-healthcheck"]
ENTRYPOINT ["/oxilytics"]
