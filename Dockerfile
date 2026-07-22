FROM node:22-alpine AS web-builder
WORKDIR /src/web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/internal/adminui/dist ./internal/adminui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scheduler-bot ./cmd/bot \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scheduler-admin ./cmd/admin \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scheduler-sync ./cmd/sync

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ENV TZ=Europe/Moscow
COPY --from=go-builder /out/ /app/
USER 65532:65532
CMD ["/app/scheduler-bot"]
