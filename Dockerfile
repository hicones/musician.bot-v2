# Stage 1: Build
FROM golang:alpine AS builder

WORKDIR /app

# Install git, ca-certificates and tzdata
RUN apk add --no-cache git ca-certificates tzdata

ENV GOPROXY=https://proxy.golang.org,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/musician-bot ./cmd/bot

# Stage 2: Runtime
FROM alpine:latest AS runner

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/musician-bot /app/musician-bot
COPY --from=builder /app/assets /app/assets

# Create data directory for SQLite persistence
RUN mkdir -p /app/data

ENV DATABASE_PATH=/app/data/database.sqlite \
    BANNER_PATH=/app/assets/banner.png \
    PLACEHOLDER_PATH=/app/assets/placeholder.png

ENTRYPOINT ["/app/musician-bot"]
