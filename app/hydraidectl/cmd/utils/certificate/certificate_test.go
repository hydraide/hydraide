package certificate

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCertificateGenerate_FilesAll(t *testing.T) {

	cert := New(
		"test.hydraide.local",
		[]string{"hydraide.local", "api.hydraide.local"},
		[]string{"192.168.1.10", "10.0.0.1"},
	)

	err := cert.Generate()
	assert.NoError(t, err, "certificate generation should not return an error")

	caCRT, caKEY, srvCRT, srvKEY, cliCRT, cliKEY := cert.Files()

	// all files should be generated
	assert.FileExists(t, caCRT, "missing CA cert: "+caCRT)
	assert.FileExists(t, caKEY, "missing CA key: "+caKEY)
	assert.FileExists(t, srvCRT, "missing server cert: "+srvCRT)
	assert.FileExists(t, srvKEY, "missing server key: "+srvKEY)
	assert.FileExists(t, cliCRT, "missing client cert: "+cliCRT)
	assert.FileExists(t, cliKEY, "missing client key: "+cliKEY)

	// check crypto
	caCert := mustLoadCert(t, caCRT)
	assert.True(t, caCert.IsCA, "CA cert must have IsCA=true")

	serverCert := mustLoadCert(t, srvCRT)
	assert.Contains(t, serverCert.ExtKeyUsage, x509.ExtKeyUsageServerAuth, "server cert must have ServerAuth EKU")
	// SAN-ok
	assert.Contains(t, serverCert.DNSNames, "hydraide.local")
	assert.Contains(t, serverCert.DNSNames, "api.hydraide.local")
	foundIP1 := false
	for _, ip := range serverCert.IPAddresses {
		if ip.String() == "192.168.1.10" {
			foundIP1 = true
			break
		}
	}
	assert.True(t, foundIP1, "server cert must contain IP SAN 192.168.1.10")

	clientCert := mustLoadCert(t, cliCRT)
	assert.Contains(t, clientCert.ExtKeyUsage, x509.ExtKeyUsageClientAuth, "client cert must have ClientAuth EKU")
}

// --- helpers ---
func mustLoadCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read cert %s: %v", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("invalid PEM in %s", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse cert %s: %v", path, err)
	}
	return cert
}
