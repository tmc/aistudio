# Multi-stage build for AIStudio
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty) -X main.BuildTime=$(date -u +%Y%m%d-%H%M%S)" \
    -o aistudio ./cmd/aistudio

# Runtime stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    ffmpeg \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 aistudio && \
    adduser -D -u 1000 -G aistudio aistudio

# Create necessary directories
RUN mkdir -p /home/aistudio/.aistudio/history && \
    chown -R aistudio:aistudio /home/aistudio

# Copy binary from builder
COPY --from=builder /build/aistudio /usr/local/bin/aistudio

# Set user
USER aistudio
WORKDIR /home/aistudio

# Environment variables
ENV AISTUDIO_HISTORY_DIR=/home/aistudio/.aistudio/history \
    AISTUDIO_CONFIG_DIR=/home/aistudio/.aistudio \
    TERM=xterm-256color

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD aistudio --version || exit 1

# Default command
ENTRYPOINT ["aistudio"]
CMD ["--help"]