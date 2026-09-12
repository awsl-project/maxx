# Multi-stage build for maxx

# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

WORKDIR /app/web

# Copy frontend package files
COPY web/package.json ./

# Install frontend dependencies
RUN pnpm install

# Copy frontend source
COPY web/ ./

# Build args for version info
ARG VITE_COMMIT=unknown

# Build frontend
RUN VITE_COMMIT=${VITE_COMMIT} pnpm build

# Stage 2: Build backend
FROM golang:1.26-alpine AS backend-builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build args for version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build backend binary with version info
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X github.com/awsl-project/maxx/internal/version.Version=${VERSION} \
    -X github.com/awsl-project/maxx/internal/version.Commit=${COMMIT} \
    -X github.com/awsl-project/maxx/internal/version.BuildTime=${BUILD_TIME}" \
    -o maxx cmd/maxx/main.go

# Stage 3: Final runtime image
FROM alpine:latest

# Install runtime dependencies (tzdata for timezone support)
RUN apk add --no-cache ca-certificates tzdata tar

# Bundle sing-box so vmess/vless/trojan/ss node links work in the official image.
ARG TARGETARCH=amd64
ARG SING_BOX_VERSION=1.14.0
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) SING_BOX_ARCH="amd64"; SING_BOX_SHA256="d2d6b4543d850269214ced70ffe41b13b1595baa1b6f9c016466abfba162c4d4" ;; \
      arm64) SING_BOX_ARCH="arm64"; SING_BOX_SHA256="1811c446a4957edee1b62ed2363607f8e99e1f7b6d88179719251d7ed5f30169" ;; \
      *) echo "unsupported sing-box arch: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    wget -O /tmp/sing-box.tar.gz "https://github.com/SagerNet/sing-box/releases/download/v${SING_BOX_VERSION}/sing-box-${SING_BOX_VERSION}-linux-${SING_BOX_ARCH}-musl.tar.gz"; \
    echo "${SING_BOX_SHA256}  /tmp/sing-box.tar.gz" | sha256sum -c -; \
    mkdir -p /tmp/sing-box /app/bin; \
    tar -xzf /tmp/sing-box.tar.gz -C /tmp/sing-box --strip-components=1; \
    install -m 0755 /tmp/sing-box/sing-box /app/bin/sing-box; \
    /app/bin/sing-box version; \
    rm -rf /tmp/sing-box /tmp/sing-box.tar.gz

WORKDIR /app

# Copy binary from backend builder
COPY --from=backend-builder /app/maxx .

# Copy built frontend from frontend builder
COPY --from=frontend-builder /app/web/dist ./web/dist

# Create directory for data (database, logs, etc.)
RUN mkdir -p /data

# Expose port
EXPOSE 9880

# Run the application
CMD ["./maxx", "-addr", ":9880", "-data", "/data"]
