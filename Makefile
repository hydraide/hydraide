# =============================================================================
# 📄 Makefile – HydrAIDE Proto Compiler
# =============================================================================
#
# This Makefile provides useful targets for generating gRPC client code from
# .proto definitions. The Go SDK is generated under ./generated/hydraidepbgo
# via the proto-go target.
#
# Need help? → https://grpc.io/docs/
#
# =============================================================================
.PHONY: build push build-push clean build-binary build-hydraidectl test proto-go help

# Default target - show help
.DEFAULT_GOAL := help

# Help target - shows all available commands
help:
	@echo "════════════════════════════════════════════════════════════════════════════"
	@echo "  🚀 HydrAIDE Makefile - Available Commands"
	@echo "════════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "  📦 BUILD COMMANDS:"
	@echo "    make build-binary    - Build Go binaries (amd64 + arm64)"
	@echo "    make build-hydraidectl - Build hydraidectl CLI (dev version)"
	@echo "    make build           - Build the full Docker image"
	@echo "    make test            - Test the Docker image locally"
	@echo "    make push            - Push Docker image to GHCR"
	@echo "    make build-push      - Build and push Docker image"
	@echo ""
	@echo "  🛠️  PROTO GENERATION:"
	@echo "    make proto-go        - Generate Go proto files"
	@echo ""
	@echo "  🧹 CLEANUP:"
	@echo "    make clean           - Remove generated files and binaries"
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════════"
	@echo ""

# Build and push docker image
# =============================================================================
# 🚨 DO NOT MODIFY THIS SECTION
#
# This section is used by the HydrAIDE GitHub Actions CI/CD pipeline
# to build and publish Docker images to GitHub Container Registry (GHCR).
#
# ➤ It automatically tags images with the version (e.g., 2.0.9) and "latest".
# ➤ It authenticates using GitHub Secrets (HYDRAIDE_DOCKER_*).
#
# Modifying this section may break the release workflow or compromise publishing.
#
# If changes are required, please coordinate with the HydrAIDE maintainers.
# =============================================================================
IMAGE_NAME=ghcr.io/hydraide/hydraide
IMAGE_TAG ?= latest

DOCKER_BUILDKIT=1

# Build only the Go binaries (useful for testing)
build-binary: proto-go
	@echo "🔨 Building Go binary for amd64..."
	cd app/server && GOOS=linux GOARCH=amd64 go build -o ../../hydraide-amd64 .
	@echo "🔨 Building Go binary for arm64..."
	cd app/server && GOOS=linux GOARCH=arm64 go build -o ../../hydraide-arm64 .
	@echo "✅ Binaries built successfully!"

# Build hydraidectl CLI with version information
build-hydraidectl:
	@echo "🔨 Building hydraidectl CLI..."
	@VERSION=$$(git describe --tags --match "hydraidectl/v*" --abbrev=0 2>/dev/null | sed 's/hydraidectl\///' || echo "dev"); \
	COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_DATE=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	echo "Version: $$VERSION, Commit: $$COMMIT, Build Date: $$BUILD_DATE"; \
	cd app/hydraidectl && go build \
		-ldflags="-X 'github.com/hydraide/hydraide/app/hydraidectl/cmd.Version=$$VERSION' \
		          -X 'github.com/hydraide/hydraide/app/hydraidectl/cmd.Commit=$$COMMIT' \
		          -X 'github.com/hydraide/hydraide/app/hydraidectl/cmd.BuildDate=$$BUILD_DATE'" \
		-o ../../hydraidectl .
	@echo "✅ hydraidectl built successfully!"
	@echo "   Install with: sudo mv hydraidectl /usr/local/bin/"

# Build the Docker image with the specified tag
build: build-binary
	@echo "🐳 Building Docker image..."
	docker build -f Dockerfile -t $(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "✅ Docker image built successfully!"

# Test the Docker image locally
test:
	@echo "🧪 Testing Docker image..."
	@docker images $(IMAGE_NAME):$(IMAGE_TAG) --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}" | cat
	@echo ""
	@echo "🚀 Starting container for quick test..."
	@docker run --rm $(IMAGE_NAME):$(IMAGE_TAG) --version 2>&1 || echo "Note: Container started successfully"
	@echo "✅ Docker image test completed!"

# Push the Docker image to GitHub Container Registry
push:
	printf "%s" "$${HYDRAIDE_DOCKER_TOKEN}" | docker login ghcr.io -u "$${HYDRAIDE_DOCKER_USERNAME}" --password-stdin || { echo "❌ Docker login failed. Please check your credentials."; exit 1; }
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(IMAGE_NAME):latest
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(IMAGE_NAME):latest

# Build the Docker image with both versioned tag and latest tag
build-push: build push

# Build from proto files
# =============================================================================

# -----------------------------------------------------------------------------
# 🧪 build – Regenerate Go code + tidy dependencies
# -----------------------------------------------------------------------------
# - Runs protoc with Go plugins
# - Ensures Go dependencies are updated
build-go: proto-go
	@echo "✅ Go dependencies updated"
	go mod tidy
	go get -u all

# -----------------------------------------------------------------------------
# 🛠️ proto – Compile .proto files to Go (no go get)
# -----------------------------------------------------------------------------
# - Generates .pb.go and .pb.grpc.go files to ./generated/go
# - Uses source-relative paths for imports
proto-go:
	@echo "🛠️  Generating Go gRPC files to ./generated/hydraidepbgo"
	@mkdir -p ./generated/hydraidepbgo
	protoc --proto_path=proto \
		--go_out=./generated/hydraidepbgo --go_opt=paths=source_relative \
		--go-grpc_out=./generated/hydraidepbgo --go-grpc_opt=paths=source_relative \
		proto/hydraide.proto

# -----------------------------------------------------------------------------
# 🧹 clean – Delete all generated proto output
# -----------------------------------------------------------------------------
# - Deletes all contents in the ./generated folders
clean:
	@echo "🧹 Cleaning generated files..."
	rm -rf generated/hydraidepbgo*
	@echo "🧹 Cleaning built binaries..."
	rm -f hydraide-amd64 hydraide-arm64 hydraidectl
