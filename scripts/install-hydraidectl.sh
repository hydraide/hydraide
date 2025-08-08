#!/bin/bash
set -e

VERSION=$(curl -s https://api.github.com/repos/hydraide/hydraide/releases/latest | grep tag_name | cut -d '"' -f4)
ARCH=$(uname -m)
DEST="/usr/local/bin/hydraidectl"

if [[ "$ARCH" == "x86_64" ]]; then
  FILE="hydraidectl-linux-amd64"
elif [[ "$ARCH" == "aarch64" ]]; then
  FILE="hydraidectl-linux-arm64"
else
  echo "❌ Unsupported architecture: $ARCH"
  exit 1
fi

echo "🔽 Downloading $FILE ($VERSION)..."
curl -L -o hydraidectl "https://github.com/hydraide/hydraide/releases/download/$VERSION/$FILE"

chmod +x hydraidectl
sudo mv hydraidectl $DEST

echo "✅ Installed to $DEST"
