# 1. frontend
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. backend
FROM golang:1.26-alpine AS build
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/oxisoft/oxilytics/internal/version.Version=${VERSION} -X github.com/oxisoft/oxilytics/internal/version.Commit=${COMMIT} -X github.com/oxisoft/oxilytics/internal/version.BuildTime=${BUILD_TIME}" \
    -o /oxilytics ./cmd/server

# 3. runtime
FROM gcr.io/distroless/static:nonroot
COPY --from=build /oxilytics /oxilytics
USER nonroot
VOLUME ["/data"]
EXPOSE 8080
ENV OXI_ADDR=0.0.0.0:8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/oxilytics", "-healthcheck"]
ENTRYPOINT ["/oxilytics"]
