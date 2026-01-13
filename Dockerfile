# Multi-stage build for maxx-next

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
FROM golang:1.25-alpine AS backend-builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev git

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
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-s -w \
    -X github.com/Bowl42/maxx-next/internal/version.Version=${VERSION} \
    -X github.com/Bowl42/maxx-next/internal/version.Commit=${COMMIT} \
    -X github.com/Bowl42/maxx-next/internal/version.BuildTime=${BUILD_TIME}" \
    -o maxx cmd/maxx/main.go

# Stage 3: Final runtime image
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs

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
