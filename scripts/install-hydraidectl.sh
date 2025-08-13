#!/bin/bash
set -euo pipefail

# ------------------------------------------------------
# 🧠 Purpose:
# This script installs the latest *hydraidectl* binary
# from GitHub releases, but **only** from tags starting
# with `hydraidectl/` (ignores `server/` releases).
#
# Works on Linux (x86_64 and ARM64).
# ------------------------------------------------------

# Detect system architecture
ARCH=$(uname -m)
DEST="/usr/local/bin/hydraidectl"

# Select binary name based on architecture
if [[ "$ARCH" == "x86_64" ]]; then
  FILE="hydraidectl-linux-amd64"
elif [[ "$ARCH" == "aarch64" ]]; then
  FILE="hydraidectl-linux-arm64"
else
  echo "❌ Unsupported architecture: $ARCH"
  exit 1
fi

# ------------------------------------------------------
# 1️⃣ Try to find the latest release from the GitHub
#    "releases" API, filtering only tags like:
#    hydraidectl/vX.Y.Z
# ------------------------------------------------------
VERSION=$(curl -fsSL 'https://api.github.com/repos/hydraide/hydraide/releases?per_page=100' \
  | grep -oE '"tag_name":\s*"hydraidectl/v[0-9]+\.[0-9]+\.[0-9]+([^"]*)?"' \
  | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/' \
  | sort -V \
  | tail -n 1 || true)

# ------------------------------------------------------
# 2️⃣ Fallback: if no matching release is found,
#    use the GitHub "tags" API instead.
# ------------------------------------------------------
if [[ -z "${VERSION:-}" ]]; then
  VERSION=$(curl -fsSL 'https://api.github.com/repos/hydraide/hydraide/tags?per_page=200' \
    | grep -oE '"name":\s*"hydraidectl/v[0-9]+\.[0-9]+\.[0-9]+([^"]*)?"' \
    | sed -E 's/.*"name":\s*"([^"]+)".*/\1/' \
    | sort -V \
    | tail -n 1 || true)
fi

# ------------------------------------------------------
# If still no match → exit with error
# ------------------------------------------------------
if [[ -z "${VERSION:-}" ]]; then
  echo "❌ Could not find latest hydraidectl release (no hydraidectl/* tags found)"
  exit 1
fi

# Construct download URL for the binary
URL="https://github.com/hydraide/hydraide/releases/download/$VERSION/$FILE"
echo "🔽 Downloading $FILE ($VERSION) from $URL"

# Download the binary
curl -fL -o hydraidectl "$URL"

# Make it executable
chmod +x hydraidectl

# Move to /usr/local/bin for global access
sudo mv hydraidectl "$DEST"

echo "✅ Installed to $DEST"
