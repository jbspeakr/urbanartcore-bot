# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.25.6-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for versioning
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_TIME}" \
    -o uac-socializer \
    .

# Verify the binary
RUN chmod +x uac-socializer && ./uac-socializer --help

# Final stage
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /build/uac-socializer /uac-socializer

# Copy runtime data
COPY --from=builder /build/data/posts_by_date.json /data/posts_by_date.json
COPY --from=builder /build/internal/infrastructure/prompts /prompts

# Set environment variables
ENV UAC_DATA_PATH=/data/posts_by_date.json
ENV UAC_PROMPTS_PATH=/prompts
ENV TZ=UTC

# OCI labels
LABEL org.opencontainers.image.title="UAC Socializer"
LABEL org.opencontainers.image.description="Automated social media publishing for Urban Art Core anniversaries"
LABEL org.opencontainers.image.authors="Brennen Stuhl"
LABEL org.opencontainers.image.url="https://forgejo.b7l.de/b7l/uac-socializer-v2"
LABEL org.opencontainers.image.source="https://forgejo.b7l.de/b7l/uac-socializer-v2"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"

# Entrypoint and default command
ENTRYPOINT ["/uac-socializer"]
CMD ["publish", "--dry-run"]
