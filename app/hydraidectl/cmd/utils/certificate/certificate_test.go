package certificate

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCertificateGenerate(t *testing.T) {

	cert := New("test.hydraide.local", []string{"hydraide.local", "api.hydraide.local"}, []string{"192.168.1.10", "10.0.0.1"})
	err := cert.Generate()
	assert.NoError(t, err, "Certificate generation should not return an error")

	clientCrt, serverCrt, serverKey := cert.Files()
	assert.FileExists(t, clientCrt, "File should exist: "+clientCrt)
	assert.FileExists(t, serverCrt, "File should exist: "+serverCrt)
	assert.FileExists(t, serverKey, "File should exist: "+serverKey)

}
