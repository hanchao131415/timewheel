# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /timewheel ./cmd/server

# Final stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata sqlite-libs

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /timewheel /app/timewheel
COPY --from=builder /app/configs /app/configs

# Create directories for data and logs
RUN mkdir -p /app/data /app/logs && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/live || exit 1

ENTRYPOINT ["/app/timewheel"]
CMD ["-config", "/app/configs/config.yaml"]
