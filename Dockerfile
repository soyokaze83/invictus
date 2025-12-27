FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o ingestion ./cmd/ingestion
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Copy binaries from builder
COPY --from=builder /app/ingestion .
COPY --from=builder /app/server .

# Expose ports for ingestion (8081) and server (8082)
EXPOSE 8081 8082

# Default command (can be overridden in docker-compose)
CMD ["./ingestion"]
