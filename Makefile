# =============================================================================
# 📄 Makefile – HydrAIDE Proto Compiler
# =============================================================================
#
# This Makefile provides useful targets for generating gRPC client code from
# .proto definitions. It supports Go out-of-the-box and allows optional
# generation for Python, Node.js, Rust, Java, and C# if tools are installed.
#
# Note:
# - Go SDK is pre-generated under ./generated/go
# - All other languages must be generated manually
#
# Safe to run in CI/CD – missing tools will not break execution
#
# Need help? → https://grpc.io/docs/
#
# =============================================================================
.PHONY: build push build-push clean build-go proto-go proto-python proto-node proto-rust proto-java proto-csharp help

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

# Build the Docker image with the specified tag
build:
	docker build -f Dockerfile -t $(IMAGE_NAME):$(IMAGE_TAG) .

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
	rm -rf generated/hydraidepbgo* generated/hydraidepbpy/* generated/hydraidepbjs/* generated/hydraidepbrs/* generated/hydraidepbjv/* generated/hydraidepbcs/*

# -----------------------------------------------------------------------------
# 🔹 proto-python – Generate Python client bindings (if grpc_tools available)
# -----------------------------------------------------------------------------
# Output: ./generated/python
proto-python:
	@echo "🐍 Syncing python dependencies via uv...\n"
	cd sdk/python/hydraidepy && \
	    uv sync
	@echo "🐍 Generating Python gRPC files...\n"
		sdk/python/hydraidepy/.venv/bin/python -m grpc_tools.protoc -I proto \
		--python_out=sdk/python/hydraidepy/src/hydraidepy/generated \
		--grpc_python_out=sdk/python/hydraidepy/src/hydraidepy/generated \
		proto/hydraide.proto

# -----------------------------------------------------------------------------
# 🔹 proto-node – Generate Node.js client bindings (requires grpc_tools_node_protoc_plugin)
# -----------------------------------------------------------------------------
# Output: ./generated/node
proto-node:
	@echo "🟨 Generating Node.js gRPC files..."
	@command -v protoc-gen-grpc >/dev/null 2>&1 || { echo "⚠️  Node.js gRPC plugin not found – skipping"; exit 0; }
	@protoc --proto_path=proto \
		--js_out=import_style=commonjs,binary:generated/hydraidepbjs \
		--grpc_out=generated/hydraidepbjs \
		proto/hydraide.proto

# -----------------------------------------------------------------------------
# 🔹 proto-rust – Generate Rust proto files (requires protoc-gen-prost)
# -----------------------------------------------------------------------------
# Output: ./generated/rust
proto-rust:
	@echo "🦀 Generating Rust proto files..."
	@command -v protoc-gen-prost >/dev/null 2>&1 || { echo "⚠️  protoc-gen-prost not installed – skipping"; exit 0; }
	@protoc --proto_path=proto \
		--prost_out=./generated/hydraidepbrs \
		proto/hydraide.proto

# -----------------------------------------------------------------------------
# 🔹 proto-java – Generate Java proto files
# -----------------------------------------------------------------------------
# Output: ./generated/java
proto-java:
	@echo "☕ Generating Java proto files..."
	@protoc --proto_path=proto \
		--java_out=./generated/hydraidepbjv \
		--grpc-java_out=./generated/hydraidepbjv \
		proto/hydraide.proto

# -----------------------------------------------------------------------------
# 🔹 proto-csharp – Generate C# (.NET) proto files
# -----------------------------------------------------------------------------
# Output: ./generated/csharp
proto-csharp:
	@echo "🎯 Generating C# proto files..."
	@protoc --proto_path=proto \
		--csharp_out=./generated/hydraidepbcs \
		--grpc_out=./generated/hydraidepbcs \
		proto/hydraide.proto

# -----------------------------------------------------------------------------
# 📋 help – List all available make targets
# -----------------------------------------------------------------------------
help:
	@echo "📦 HydrAIDE Proto Makefile – Available commands:"
	@echo ""
	@echo "🔧 build       	– build Docker image with latest Server code"
	@echo "📤 push        	– Push Docker image to GitHub Container Registry"
	@echo "🔄 build-push  	– Build and push Docker image to GitHub Container Registry"
	@echo "🔨 build-go       	– Compile proto to Go and tidy dependencies"
	@echo "🧠 proto-go       	– Only generate Go bindings"
	@echo "🐍 proto-python   	– Generate Python gRPC code (if tools exist)"
	@echo "🟨 proto-node     	– Generate Node.js gRPC code (if tools exist)"
	@echo "🦀 proto-rust     	– Generate Rust proto files (requires protoc-gen-prost)"
	@echo "☕ proto-java     	– Generate Java gRPC bindings"
	@echo "🎯 proto-csharp   	– Generate C#/.NET gRPC bindings"
	@echo "🧹 clean          	– Remove all generated proto code"
	@echo ""
	@echo "🧭 Notes:"
	@echo " - No plugins? No problem. Targets will skip gracefully."
	@echo " - Generated code goes into ./generated/<language>"
