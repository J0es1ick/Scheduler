FROM node:22.16.0-alpine AS admin-web-builder
WORKDIR /src/web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

FROM node:22.16.0-alpine AS site-web-builder
WORKDIR /src/web/site
COPY web/site/package.json web/site/package-lock.json ./
RUN npm ci
COPY web/site/ ./
RUN npm run build

FROM golang:1.25.12-alpine AS go-builder
ARG VERSION=dev
ARG COMMIT=local
ARG BUILD_TIME=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=admin-web-builder /src/internal/adminui/dist ./internal/adminui/dist
COPY --from=site-web-builder /src/internal/siteui/dist ./internal/siteui/dist
RUN LDFLAGS="-s -w -X github.com/J0es1ick/Scheduler/internal/buildinfo.Version=${VERSION} -X github.com/J0es1ick/Scheduler/internal/buildinfo.Commit=${COMMIT} -X github.com/J0es1ick/Scheduler/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    && CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o /out/scheduler-bot ./cmd/bot \
    && CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o /out/scheduler-admin ./cmd/admin \
    && CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o /out/scheduler-site ./cmd/site \
    && CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o /out/scheduler-sync ./cmd/sync \
    && CGO_ENABLED=0 go build -trimpath -ldflags="$LDFLAGS" -o /out/scheduler-connector ./cmd/connector

FROM alpine:3.24
ARG VERSION=dev
ARG COMMIT=local
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="Scheduler" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$BUILD_TIME" \
      org.opencontainers.image.source="https://github.com/J0es1ick/Scheduler"
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ENV TZ=Europe/Moscow
COPY --from=go-builder /out/ /app/
USER 65532:65532
CMD ["/app/scheduler-bot"]
