FROM golang:1.25 AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build both binaries (CGO enabled for ONNX Runtime support)
RUN CGO_ENABLED=1 GOOS=linux go build -o ingestion ./cmd/ingestion
RUN CGO_ENABLED=1 GOOS=linux go build -o server ./cmd/server

# ONNX Runtime downloader stage
FROM debian:bookworm-slim AS onnx-downloader

ARG ONNX_VERSION=1.23.2
ARG TARGETARCH

WORKDIR /tmp

# Download ONNX Runtime based on architecture
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates && \
    if [ "$TARGETARCH" = "arm64" ]; then \
        ONNX_ARCH="aarch64"; \
    else \
        ONNX_ARCH="x64"; \
    fi && \
    wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-linux-${ONNX_ARCH}-${ONNX_VERSION}.tgz" && \
    tar -xzf "onnxruntime-linux-${ONNX_ARCH}-${ONNX_VERSION}.tgz" && \
    mv onnxruntime-linux-*/lib/* /tmp/ && \
    rm -rf /var/lib/apt/lists/*

# Final stage - use Debian for glibc compatibility with ONNX Runtime
FROM debian:bookworm-slim

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime libraries
COPY --from=onnx-downloader /tmp/libonnxruntime.so* /usr/local/lib/

# Update library cache
RUN ldconfig

# Copy binaries from builder
COPY --from=builder /app/ingestion .
COPY --from=builder /app/server .

# Expose ports for ingestion (8081) and server (8082)
EXPOSE 8081 8082

# Default command (can be overridden in docker-compose)
CMD ["./ingestion"]
