# ==========================================
# Stage 1: Build binary
# ==========================================
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install ca-certificates and git
RUN apk add --no-cache ca-certificates git

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static production binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o kinjo-api ./cmd/api

# ==========================================
# Stage 2: Minimal Production Runtime
# ==========================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S kinjo && adduser -S kinjo -G kinjo

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/kinjo-api /app/kinjo-api

# Create directory for uploads if using local storage driver
RUN mkdir -p /app/uploads && chown -R kinjo:kinjo /app

USER kinjo:kinjo

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/api/healthz || exit 1

ENTRYPOINT ["/app/kinjo-api"]
