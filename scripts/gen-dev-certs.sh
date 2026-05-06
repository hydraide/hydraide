#!/bin/sh
# gen-certs.sh — generate development TLS material for the example tree.
#
# Runs once on `docker compose up`. Idempotent: if all expected files already
# exist, exits without changes. Produces a CA, a server certificate signed by
# the CA, and a client certificate signed by the same CA. Output goes to the
# bind-mounted /certs directory which is also mounted into the hydraide
# service and into the host filesystem at docs/sdk/go/examples/certificate/.
#
# This is for local development against an in-tree HydrAIDE instance only.
# Do not use these certificates in production.

set -e

CERT_DIR="${CERT_DIR:-/certs}"
SERVER_CN="${SERVER_CN:-hydraide}"
CLIENT_CN="${CLIENT_CN:-hydraide-client}"
DAYS="${DAYS:-3650}"

cd "$CERT_DIR"

if [ -f ca.crt ] && [ -f server.crt ] && [ -f server.key ] && [ -f client.crt ] && [ -f client.key ]; then
    echo "certificates already present in $CERT_DIR — skipping"
    exit 0
fi

apk add --no-cache openssl >/dev/null 2>&1 || true

cat > openssl.cnf <<'EOF'
[req]
distinguished_name = req_distinguished_name
req_extensions     = v3_req
prompt             = no

[req_distinguished_name]
C  = HU
O  = HydrAIDE
OU = Examples
CN = placeholder

[v3_req]
basicConstraints = CA:FALSE
keyUsage         = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName   = @alt_names

[alt_names]
DNS.1 = hydraide
DNS.2 = localhost
IP.1  = 127.0.0.1
EOF

echo "generating CA"
openssl genrsa -out ca.key 4096 2>/dev/null
openssl req -x509 -new -nodes -key ca.key -sha256 -days "$DAYS" \
    -subj "/C=HU/O=HydrAIDE/OU=CA/CN=HydrAIDE Examples CA" \
    -out ca.crt 2>/dev/null

echo "generating server certificate (CN=$SERVER_CN)"
openssl genrsa -out server.key 4096 2>/dev/null
openssl req -new -key server.key \
    -subj "/C=HU/O=HydrAIDE/OU=Server/CN=$SERVER_CN" \
    -reqexts v3_req -config openssl.cnf \
    -out server.csr 2>/dev/null
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days "$DAYS" -sha256 \
    -extensions v3_req -extfile openssl.cnf \
    -out server.crt 2>/dev/null

echo "generating client certificate (CN=$CLIENT_CN)"
openssl genrsa -out client.key 4096 2>/dev/null
openssl req -new -key client.key \
    -subj "/C=HU/O=HydrAIDE/OU=Client/CN=$CLIENT_CN" \
    -reqexts v3_req -config openssl.cnf \
    -out client.csr 2>/dev/null
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days "$DAYS" -sha256 \
    -extensions v3_req -extfile openssl.cnf \
    -out client.crt 2>/dev/null

rm -f server.csr client.csr ca.srl openssl.cnf

# 644 on .key as well: these are local development certs generated into
# a bind-mounted volume that is consumed by both the container (running
# as root) and host-side processes (CI runner, current user, …). 600
# would lock out everyone but the file's owner, which is root under
# rootful Docker — not what we want for a dev tree. For production
# certs, generate via your own PKI with stricter permissions.
chmod 644 ca.crt server.crt client.crt ca.key server.key client.key

echo "done. certificates written to $CERT_DIR:"
ls -1 "$CERT_DIR"
