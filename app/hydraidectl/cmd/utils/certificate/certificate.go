package certificate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Certificate defines the interface for generating and retrieving
// file paths of TLS certificates (CA, server, and client).
// Implementations must provide:
//   - Generate(): create new certificates and keys
//   - Files(): return file paths for all generated certificate and key files
type Certificate interface {
	Generate() error
	Files() (caCRT, caKEY, serverCRT, serverKEY, clientCRT, clientKEY string)
}

// certificate is the internal implementation of the Certificate interface.
// It stores configuration parameters (name, DNS, IP addresses)
// and resolved file paths for generated certificates and keys.
type certificate struct {
	name string   // Common name or identifier used in certificates
	dns  []string // DNS Subject Alternative Names (SANs) for the server certificate
	ip   []string // IP SANs for the server certificate

	tempDir   string // Temporary directory where certificates/keys are stored
	caCRT     string // File path for the CA certificate
	caKEY     string // File path for the CA private key
	serverCRT string // File path for the server certificate
	serverKEY string // File path for the server private key
	clientCRT string // File path for the client certificate
	clientKEY string // File path for the client private key
}

// New creates a new Certificate instance with the given name, DNS, and IP SANs.
// Certificates and keys will be generated under the system's temporary directory.
//
// Example:
//
//	cert := New("my-service", []string{"example.com"}, []string{"127.0.0.1"})
//
// This prepares the file paths, but no certificates are generated
// until Generate() is called.
func New(name string, dns []string, ip []string) Certificate {
	td := os.TempDir()
	return &certificate{
		name:      name,
		dns:       dns,
		ip:        ip,
		tempDir:   td,
		caCRT:     filepath.Join(td, "ca.crt"),
		caKEY:     filepath.Join(td, "ca.key"),
		serverCRT: filepath.Join(td, "server.crt"),
		serverKEY: filepath.Join(td, "server.key"),
		clientCRT: filepath.Join(td, "client.crt"),
		clientKEY: filepath.Join(td, "client.key"),
	}
}

// Generate creates a complete set of TLS certificates and keys
// for HydrAIDE usage. It generates three certificate types:
//
//  1. Certificate Authority (CA)
//     - Self-signed root certificate used for signing others
//     - Long lifespan (10 years)
//     - Stored in: ca.crt / ca.key
//
//  2. Server Certificate
//     - Signed by the CA
//     - Includes DNS and IP SANs
//     - Intended for server-side TLS (5 years)
//     - Stored in: server.crt / server.key
//
//  3. Client Certificate
//     - Signed by the CA
//     - Intended for mTLS authentication
//     - CN-based identification, no SANs by default (5 years)
//     - Stored in: client.crt / client.key
//
// On success, all certificates and keys are written into the
// temporary directory defined in the certificate struct.
// Any error (generation, signing, writing to disk) aborts the process.
func (c *certificate) Generate() error {
	// ===== 1) CA (Root Certificate Authority) =====
	// Generate a private key for the CA
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}

	// Define a CA certificate template
	caTemplate := &x509.Certificate{
		SerialNumber:          mustRandSerial(),
		Subject:               pkix.Name{Country: []string{"HU"}, Organization: []string{"HydrAIDE"}, OrganizationalUnit: []string{"CA"}, CommonName: "HydrAIDE Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),   // valid slightly before generation
		NotAfter:              time.Now().AddDate(10, 0, 0), // valid for 10 years
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	// Self-sign the CA certificate
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}
	// Save CA certificate and key to disk
	if err := writeCert(c.caCRT, caDER); err != nil {
		return err
	}
	if err := writeKey(c.caKEY, caKey); err != nil {
		return err
	}

	// ===== 2) SERVER Certificate =====
	// Generate a private key for the server
	srvKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}

	// Define the server certificate template
	srvTpl := &x509.Certificate{
		SerialNumber:          mustRandSerial(),
		Subject:               pkix.Name{Country: []string{"HU"}, Organization: []string{"HydrAIDE"}, OrganizationalUnit: []string{"Server"}, CommonName: c.name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0), // valid for 5 years
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Add DNS SANs if provided
	for _, d := range c.dns {
		srvTpl.DNSNames = append(srvTpl.DNSNames, d)
	}
	// Add IP SANs if provided
	for _, ip := range c.ip {
		if parsed := net.ParseIP(ip); parsed != nil {
			srvTpl.IPAddresses = append(srvTpl.IPAddresses, parsed)
		}
	}

	// Load CA cert and key for signing
	caCert, err := readCert(c.caCRT)
	if err != nil {
		return fmt.Errorf("read ca cert: %w", err)
	}
	caPriv, err := readKey(c.caKEY)
	if err != nil {
		return fmt.Errorf("read ca key: %w", err)
	}

	// Sign the server certificate with the CA
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTpl, caCert, &srvKey.PublicKey, caPriv)
	if err != nil {
		return fmt.Errorf("create server cert: %w", err)
	}
	// Save server certificate and key to disk
	if err := writeCert(c.serverCRT, srvDER); err != nil {
		return err
	}
	if err := writeKey(c.serverKEY, srvKey); err != nil {
		return err
	}

	// ===== 3) CLIENT Certificate (for mTLS) =====
	// Generate a private key for the client
	clKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate client key: %w", err)
	}

	// Define the client certificate template
	clTpl := &x509.Certificate{
		SerialNumber:          mustRandSerial(),
		Subject:               pkix.Name{Country: []string{"HU"}, Organization: []string{"HydrAIDE"}, OrganizationalUnit: []string{"Client"}, CommonName: c.name + " Client"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0), // valid for 5 years
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		// Note: no SANs by default; CN-based filtering can be enforced on server
	}

	// Sign the client certificate with the CA
	clDER, err := x509.CreateCertificate(rand.Reader, clTpl, caCert, &clKey.PublicKey, caPriv)
	if err != nil {
		return fmt.Errorf("create client cert: %w", err)
	}
	// Save client certificate and key to disk
	if err := writeCert(c.clientCRT, clDER); err != nil {
		return err
	}
	if err := writeKey(c.clientKEY, clKey); err != nil {
		return err
	}

	// Summary output
	fmt.Println("✅ Certificates generated and saved:")
	fmt.Println(" - CA:     ", c.caCRT, c.caKEY)
	fmt.Println(" - SERVER: ", c.serverCRT, c.serverKEY)
	fmt.Println(" - CLIENT: ", c.clientCRT, c.clientKEY)

	return nil
}

// Files returns the absolute file paths of all generated
// certificates and private keys in the following order:
//
//   - CA certificate (ca.crt)
//   - CA private key (ca.key)
//   - Server certificate (server.crt)
//   - Server private key (server.key)
//   - Client certificate (client.crt)
//   - Client private key (client.key)
//
// This is a convenience method for consumers that need
// to locate the generated files without knowing the internals.
func (c *certificate) Files() (string, string, string, string, string, string) {
	return c.caCRT, c.caKEY, c.serverCRT, c.serverKEY, c.clientCRT, c.clientKEY
}

// writeCert creates a new file at the given path and writes
// a PEM-encoded X.509 certificate to it.
//
// Parameters:
//   - path: target file path (e.g., "server.crt")
//   - certBytes: DER-encoded certificate bytes
//
// The file is created or overwritten. Any failure in file
// creation or encoding is returned as an error.
func writeCert(path string, certBytes []byte) error {
	// Create or overwrite the target file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("warning: failed to close cert file %s: %v\n", path, err)
		}
	}()

	// Encode the certificate into PEM format and write it
	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
}

// writeKey creates a new file at the given path and writes
// a PEM-encoded RSA private key to it.
//
// Parameters:
//   - path: target file path (e.g., "server.key")
//   - key:  RSA private key instance
//
// Security:
//   - File permissions are set to 0600 (owner read/write only).
//   - The key is encoded in PKCS#1 format.
func writeKey(path string, key *rsa.PrivateKey) error {
	// Open (or create) the file securely with restricted permissions
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("warning: failed to close key file %s: %v\n", path, err)
		}
	}()

	// Encode the private key into PEM format
	return pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

// readCert loads and parses a PEM-encoded X.509 certificate
// from the given file path.
//
// Parameters:
//   - path: path to a PEM-formatted certificate file
//
// Returns:
//   - Parsed *x509.Certificate on success
//   - Error if the file is unreadable, not PEM, or not a certificate
func readCert(path string) (*x509.Certificate, error) {
	// Read raw file content
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Decode PEM structure
	block, _ := pem.Decode(b)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("decode cert %s: invalid PEM", path)
	}

	// Parse DER-encoded certificate bytes
	return x509.ParseCertificate(block.Bytes)
}

// readKey loads and parses a PEM-encoded RSA private key
// from the given file path.
//
// Parameters:
//   - path: path to a PEM-formatted private key file
//
// Returns:
//   - Parsed *rsa.PrivateKey on success
//   - Error if the file is unreadable, not PEM, or not an RSA private key
func readKey(path string) (*rsa.PrivateKey, error) {
	// Read raw file content
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Decode PEM structure
	block, _ := pem.Decode(b)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("decode key %s: invalid PEM", path)
	}

	// Parse PKCS#1 private key
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// mustRandSerial generates a random 128-bit serial number
// suitable for use in X.509 certificates.
//
// Behavior:
//   - On success: returns a cryptographically secure random big.Int
//   - On failure: falls back to using the current UnixNano timestamp
//     to guarantee uniqueness, but with reduced randomness.
func mustRandSerial() *big.Int {
	// Define the upper bound: 2^128
	limit := new(big.Int).Lsh(big.NewInt(1), 128)

	// Try to generate a random serial
	sn, err := rand.Int(rand.Reader, limit)
	if err != nil {
		// Fallback: use current time in nanoseconds
		return big.NewInt(time.Now().UnixNano())
	}
	return sn
}
