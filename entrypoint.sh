#!/bin/sh

PUID=${PUID:-1000}
PGID=${PGID:-1000}

# Create group and user dynamically
if ! getent group hydraide >/dev/null; then
  addgroup -g "$PGID" hydraide
fi

if ! id -u hydraide >/dev/null 2>&1; then
  adduser -D -H -u "$PUID" -G hydraide hydraide
fi

# Fix ownership of mounted folders if needed
chown -R hydraide:hydraide /hydraide

# Drop to non-root user
exec su-exec hydraide "$@"
