# Default: show help
default:
    just --list

# Show current version info
version:
    #!/bin/bash
    COMMIT=$(git rev-parse --short HEAD)
    BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
    echo "Build info:"
    echo "  Commit:  ${COMMIT}"
    echo "  Built:   ${BUILD_TIME}"

# Run all tests
test:
    @echo "Running tests..."
    go test ./...

# Run golangci-lint if available
lint:
    @echo "Running linter..."
    @if command -v golangci-lint > /dev/null; then \
        golangci-lint run ./...; \
    else \
        echo "Warning: golangci-lint is not installed. Install it from: https://golangci-lint.run/usage/install/"; \
        echo "Running basic go vet instead..."; \
        go vet ./...; \
    fi

# Format Go code with gofmt
fmt:
    @echo "Formatting Go code..."
    gofmt -s -w .
    @echo "Format complete"

# Run go vet for static analysis
vet:
    @echo "Running go vet..."
    go vet ./...

# Quality gate: formatting, linting, then tests
check: fmt vet lint test
    @echo "All checks passed!"

# Build and push to registry (REQUIRES tests to pass first)
push-image: check
    #!/bin/bash
    set -e

    # Load environment variables from .env
    if [ -f .env ]; then
        export $(cat .env | grep -v '^#' | xargs)
    else
        echo "Error: .env file not found"
        exit 1
    fi

    # Verify required environment variables
    if [ -z "$SCW_SECRET_KEY" ] || [ -z "$SCW_REGISTRY" ] || [ -z "$SCW_IMAGE_NAME" ]; then
        echo "Error: Missing required environment variables in .env:"
        echo "  SCW_SECRET_KEY: ${SCW_SECRET_KEY:-(not set)}"
        echo "  SCW_REGISTRY: ${SCW_REGISTRY:-(not set)}"
        echo "  SCW_IMAGE_NAME: ${SCW_IMAGE_NAME:-(not set)}"
        exit 1
    fi

    COMMIT=$(git rev-parse --short HEAD)
    BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
    IMAGE_TAG="${SCW_REGISTRY}/${SCW_IMAGE_NAME}:latest"

    echo "Building and pushing to ${IMAGE_TAG}"
    echo "  Version: latest"
    echo "  Commit:  ${COMMIT}"
    echo "  Built:   ${BUILD_TIME}"

    # Authenticate with registry
    echo "Authenticating with registry ${SCW_REGISTRY}..."
    echo "$SCW_SECRET_KEY" | podman login -u nologin --password-stdin "${SCW_REGISTRY}"

    # Build container image
    echo "Running: podman build --build-arg VERSION=latest --build-arg COMMIT=${COMMIT} --build-arg BUILD_TIME=${BUILD_TIME} -t ${IMAGE_TAG} ."
    podman build \
      --build-arg VERSION=latest \
      --build-arg COMMIT="${COMMIT}" \
      --build-arg BUILD_TIME="${BUILD_TIME}" \
      -t "${IMAGE_TAG}" \
      .

    # Push to registry
    podman push "${IMAGE_TAG}"
    echo "✓ Successfully pushed ${IMAGE_TAG}"

# Build and test locally (no Docker)
build-local:
    go build -o bin/uac-socializer ./main.go
    ./bin/uac-socializer --help

# OpenTofu: Initialize working directory
tofu-init:
    #!/bin/bash
    # Export .env variables: both raw (for Scaleway SDK) and TF_VAR_ prefixed (for OpenTofu)
    export $(grep -v '^#' .env | xargs)
    while IFS='=' read -r key value; do [[ "$key" =~ ^# ]] && continue; [[ -z "$key" ]] && continue; export "TF_VAR_${key}=${value}"; done < .env
    cd tofu && tofu init

# OpenTofu: Validate configuration
tofu-validate:
    #!/bin/bash
    # Export .env variables: both raw (for Scaleway SDK) and TF_VAR_ prefixed (for OpenTofu)
    export $(grep -v '^#' .env | xargs)
    while IFS='=' read -r key value; do [[ "$key" =~ ^# ]] && continue; [[ -z "$key" ]] && continue; export "TF_VAR_${key}=${value}"; done < .env
    cd tofu && tofu validate

# OpenTofu: Plan changes
tofu-plan:
    #!/bin/bash
    # Export .env variables: both raw (for Scaleway SDK) and TF_VAR_ prefixed (for OpenTofu)
    export $(grep -v '^#' .env | xargs)
    while IFS='=' read -r key value; do [[ "$key" =~ ^# ]] && continue; [[ -z "$key" ]] && continue; export "TF_VAR_${key}=${value}"; done < .env
    cd tofu && tofu plan

# OpenTofu: Apply configuration
tofu-apply:
    #!/bin/bash
    # Export .env variables: both raw (for Scaleway SDK) and TF_VAR_ prefixed (for OpenTofu)
    export $(grep -v '^#' .env | xargs)
    while IFS='=' read -r key value; do [[ "$key" =~ ^# ]] && continue; [[ -z "$key" ]] && continue; export "TF_VAR_${key}=${value}"; done < .env
    cd tofu && tofu apply

# OpenTofu: Destroy infrastructure (careful!)
tofu-destroy:
    #!/bin/bash
    echo "⚠️  WARNING: This will destroy all Scaleway resources!"
    # Export .env variables: both raw (for Scaleway SDK) and TF_VAR_ prefixed (for OpenTofu)
    export $(grep -v '^#' .env | xargs)
    while IFS='=' read -r key value; do [[ "$key" =~ ^# ]] && continue; [[ -z "$key" ]] && continue; export "TF_VAR_${key}=${value}"; done < .env
    cd tofu && tofu destroy

# Complete deployment workflow: build, push, and deploy with OpenTofu
deploy: check push-image tofu-apply
    @echo "✓ Deployment complete"
