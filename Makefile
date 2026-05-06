# =============================================================================
# Makefile - HydrAIDE local developer helpers
# =============================================================================
#
# Scope: this Makefile is for LOCAL development only.
#
# Production builds (server binaries + multi-arch Docker images, hydraidectl
# release binaries) are produced by the GitHub Actions workflows under
# .github/workflows/ and do NOT use this Makefile. Do not add release/publish
# logic here - put it in the workflows instead.
#
# What lives here:
#   - proto-go            Regenerate Go gRPC stubs from proto/hydraide.proto
#   - build-hydraidectl   Build the hydraidectl CLI locally with version info
#   - clean               Remove generated stubs and local binaries
#
# =============================================================================
.PHONY: help proto-go build-hydraidectl clean

.DEFAULT_GOAL := help

help:
	@echo "HydrAIDE Makefile - local developer targets"
	@echo ""
	@echo "  make proto-go            Generate Go gRPC stubs into ./generated/hydraidepbgo"
	@echo "  make build-hydraidectl   Build the hydraidectl CLI locally (with version ldflags)"
	@echo "  make clean               Remove generated stubs and local binaries"
	@echo ""
	@echo "Production server + hydraidectl release builds run in GitHub Actions,"
	@echo "see .github/workflows/."

# Regenerate Go gRPC stubs from proto/hydraide.proto.
# Run this whenever proto/hydraide.proto changes.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc on PATH.
proto-go:
	@echo "Generating Go gRPC stubs into ./generated/hydraidepbgo"
	@mkdir -p ./generated/hydraidepbgo
	protoc --proto_path=proto \
		--go_out=./generated/hydraidepbgo --go_opt=paths=source_relative \
		--go-grpc_out=./generated/hydraidepbgo --go-grpc_opt=paths=source_relative \
		proto/hydraide.proto

# Build hydraidectl locally with version / commit / build-date ldflags.
# Useful for testing CLI changes against a real instance before tagging a release.
# Note: official release binaries are produced by .github/workflows/cd_hydraidectl.yaml.
build-hydraidectl:
	@VERSION=$$(git describe --tags --match "hydraidectl/v*" --abbrev=0 2>/dev/null | sed 's/hydraidectl\///' || echo "dev"); \
	COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_DATE=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	echo "Building hydraidectl (version=$$VERSION commit=$$COMMIT)"; \
	cd app/hydraidectl && go build \
		-ldflags="-X 'github.com/hydraide/hydraide/app/hydraidectl/cmd.Version=$$VERSION' \
		          -X 'github.com/hydraide/hydraide/app/hydraidectl/cmd.Commit=$$COMMIT' \
		          -X 'github.com/hydraide/hydraide/app/hydraidectl/cmd.BuildDate=$$BUILD_DATE'" \
		-o ../../hydraidectl .
	@echo "Built ./hydraidectl  (install with: sudo mv hydraidectl /usr/local/bin/)"

# Remove generated proto stubs and any local hydraidectl binary.
clean:
	rm -rf generated/hydraidepbgo*
	rm -f hydraidectl
