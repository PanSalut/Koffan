# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled (required for SQLite)
RUN CGO_ENABLED=1 go build -o shopping-list .

# Production stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata libc6-compat

# Copy binary from builder
COPY --from=builder /app/shopping-list .

# Copy templates and static files
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Create data directory for database
RUN mkdir -p /data

# Set environment variables
ENV APP_ENV=production
ENV PORT=80
ENV DB_PATH=/data/shopping.db

# Expose port
EXPOSE 80

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:80/login || exit 1

# Run the application
CMD ["./shopping-list"]
