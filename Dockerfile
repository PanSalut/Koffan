# Keep the build toolchain and runtime distribution explicit so security updates
# are reviewable instead of depending on floating major-version tags.
ARG GO_VERSION=1.26.5
ARG ALPINE_VERSION=3.23

# Build stage
FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

WORKDIR /app

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Read version and build with ldflags
RUN VERSION=$(cat VERSION | tr -d '\n') && \
    CGO_ENABLED=1 go build -ldflags "-X shopping-list/handlers.AppVersion=$VERSION" -o shopping-list .

# Production stage
FROM alpine:${ALPINE_VERSION}

LABEL org.opencontainers.image.source=https://github.com/PanSalut/Koffan
LABEL org.opencontainers.image.description="Open source self-hosted groceries list for families and shared households"
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /app

# Install current runtime security updates and only the packages used by Koffan.
RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/shopping-list .

# Create data directory for database
RUN mkdir -p /data

# Set environment variables
ENV APP_ENV=production
ENV PORT=8080
ENV DB_PATH=/data/shopping.db

# Expose port
EXPOSE 8080

# Health check (uses $PORT so custom port overrides still report healthy)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider "http://127.0.0.1:${PORT}/login" || exit 1

# Run the application
CMD ["./shopping-list"]
