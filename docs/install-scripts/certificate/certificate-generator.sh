#!/bin/bash

################################################################################
#                          HydrAIDE TLS Certificate Generator
# ------------------------------------------------------------------------------
# This script generates a self-signed Certificate Authority (CA)
# and a server certificate signed by that CA, intended for use with HydrAIDE.
#
# ⚠️ REQUIRED: Customize the variables in the CONFIGURATION section below.
#
# Output files:
#   - ca.key          (CA private key – keep safe, not shared)
#   - ca.crt          (CA public certificate – must be copied to the CLIENT application)
#   - server.key      (server private key – must be copied to the HydrAIDE SERVER)
#   - server.crt      (server public certificate – must be copied to the HydrAIDE SERVER)
#
# This script will abort if any of these files already exist.
#
# 📄 Requirements:
#   - OpenSSL installed
#   - A valid `openssl.cnf` file in the working directory
#
# 📦 Deployment Instructions (after running this script):
#   1. Copy `ca.crt` to the CLIENT application → used to verify the server identity.
#   2. Copy `server.key` and `server.crt` to your HydrAIDE server's certificate folder:
#        e.g. `/path/to/hydraide/certificate/`
################################################################################

# ==============================================================================
# CONFIGURATION – CHANGE THESE VALUES BEFORE RUNNING
# ==============================================================================

# The subject field for the self-signed CA certificate.
# Replace these values with your organization's information.
CA_SUBJECT="/C=XX/ST=YourState/L=YourCity/O=Your Company/OU=IT/CN=Your Company Root CA"

# Validity duration for the certificates in days (default: 10 years)
DAYS_VALID=3650

# File names for the generated outputs (you can change them if needed)
CA_KEY="ca.key"
CA_CERT="ca.crt"
SERVER_KEY="server.key"
SERVER_CSR="server.csr"
SERVER_CERT="server.crt"
OPENSSL_CONFIG="openssl.cnf"  # must exist and contain [req] and [req_ext] sections

# ==============================================================================
# DO NOT MODIFY BELOW THIS LINE UNLESS YOU KNOW WHAT YOU’RE DOING
# ==============================================================================

# Check for existing output files
if [ -f "$CA_KEY" ] || [ -f "$CA_CERT" ] || [ -f "$SERVER_KEY" ] || [ -f "$SERVER_CERT" ]; then
  echo "❌ Some certificate files already exist. Please remove or rename them before running this script."
  exit 1
fi

# Generate CA private key
echo "🔐 Generating CA private key..."
openssl genpkey -algorithm RSA -out "$CA_KEY"

# Generate self-signed CA certificate
echo "🏷️  Generating self-signed CA certificate..."
openssl req -new -x509 -days "$DAYS_VALID" -key "$CA_KEY" -out "$CA_CERT" -subj "$CA_SUBJECT"

# Generate server private key
echo "🔐 Generating server private key..."
openssl genpkey -algorithm RSA -out "$SERVER_KEY"

# Generate Certificate Signing Request (CSR) for the server
echo "📄 Generating server CSR (certificate signing request)..."
openssl req -new -key "$SERVER_KEY" -out "$SERVER_CSR" -config "$OPENSSL_CONFIG"

# Generate and sign server certificate with CA
echo "✅ Signing the server certificate using the CA..."
openssl x509 -req -days "$DAYS_VALID" -in "$SERVER_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial -out "$SERVER_CERT" -extensions req_ext -extfile "$OPENSSL_CONFIG"

# Display result
echo "🔍 Displaying the generated server certificate:"
openssl x509 -in "$SERVER_CERT" -text -noout

echo ""
echo "✅ Certificates generated successfully."
echo "──────────────────────────────────────────────"
echo "Next steps:"
echo "📤 Copy '$CA_CERT' → into your client application (used for verifying the server)."
echo "📥 Copy '$SERVER_KEY' and '$SERVER_CERT' → into your HydrAIDE server certificate folder (e.g. /mounted-docker-folder/certificate/)"
echo ""
