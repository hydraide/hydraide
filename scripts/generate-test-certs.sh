#!/usr/bin/env bash
# Generates a self-signed CA + server + client certificate set used by the
# Go SDK's e2e tests. Output goes to sdk/go/hydraidego/testdata/certs/.
# Files are gitignored; rerun this script whenever you need a fresh set.
#
# Usage: scripts/generate-test-certs.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/sdk/go/hydraidego/testdata/certs"

mkdir -p "${OUT_DIR}"
cd "${OUT_DIR}"

echo "Generating test certificates in ${OUT_DIR}"

# 1. CA
openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout ca.key -out ca.crt \
    -days 3650 \
    -subj "/C=HU/O=HydrAIDE/OU=CA/CN=HydrAIDE Test CA" \
    >/dev/null 2>&1

# 2. Server (signed by CA, SAN with localhost + 127.0.0.1)
openssl req -newkey rsa:2048 -nodes \
    -keyout server.key -out server.csr \
    -subj "/C=HU/O=HydrAIDE/OU=Server/CN=hydraide" \
    >/dev/null 2>&1

cat >server.ext <<EOF
subjectAltName=DNS:localhost,IP:127.0.0.1
extendedKeyUsage=serverAuth
basicConstraints=critical,CA:FALSE
EOF

openssl x509 -req -in server.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 3650 \
    -extfile server.ext \
    >/dev/null 2>&1

# 3. Client (signed by CA, EKU clientAuth)
openssl req -newkey rsa:2048 -nodes \
    -keyout client.key -out client.csr \
    -subj "/C=HU/O=HydrAIDE/OU=Client/CN=hydraide-test-client" \
    >/dev/null 2>&1

cat >client.ext <<EOF
extendedKeyUsage=clientAuth
basicConstraints=critical,CA:FALSE
EOF

openssl x509 -req -in client.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out client.crt -days 3650 \
    -extfile client.ext \
    >/dev/null 2>&1

# Cleanup intermediate files.
rm -f server.csr client.csr server.ext client.ext ca.srl

chmod 600 *.key

echo "Generated:"
ls -1 "${OUT_DIR}"
