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

type Certificate interface {
	Generate() error
	Files() (clientCRT, serverCRT, serverKEY string)
}

type certificate struct {
	name      string
	dns       []string
	ip        []string
	tempDir   string
	clientCRT string
	serverCRT string
	serverKEY string
}

func New(name string, dns []string, ip []string) Certificate {
	tempDir := os.TempDir()
	return &certificate{
		name:      name,
		dns:       dns,
		ip:        ip,
		tempDir:   tempDir,
		clientCRT: filepath.Join(tempDir, "client.crt"),
		serverCRT: filepath.Join(tempDir, "server.crt"),
		serverKEY: filepath.Join(tempDir, "server.key"),
	}
}

func (c *certificate) Generate() error {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate CA private key: %w", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Country: []string{"HU"}, Organization: []string{"HydrAIDE"}, OrganizationalUnit: []string{"CLI"}, CommonName: "HydrAIDE Root CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("failed to create CA cert: %w", err)
	}

	if err := writeCert(c.clientCRT, caCertBytes); err != nil {
		return err
	}
	if err := writeKey(filepath.Join(c.tempDir, "client.key"), caKey); err != nil {
		return err
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate server private key: %w", err)
	}

	sn, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	serverTemplate := x509.Certificate{
		SerialNumber: sn,
		Subject: pkix.Name{
			Country:            []string{"HU"},
			Province:           []string{"PlaceholderState"},
			Locality:           []string{"PlaceholderCity"},
			Organization:       []string{"PlaceholderOrg"},
			OrganizationalUnit: []string{"PlaceholderUnit"},
			CommonName:         c.name,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(10, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	for _, dns := range c.dns {
		serverTemplate.DNSNames = append(serverTemplate.DNSNames, dns)
	}
	for _, ip := range c.ip {
		if parsed := net.ParseIP(ip); parsed != nil {
			serverTemplate.IPAddresses = append(serverTemplate.IPAddresses, parsed)
		}
	}

	caCert, err := readCert(c.clientCRT)
	if err != nil {
		return err
	}
	caPrivKey, err := readKey(filepath.Join(c.tempDir, "client.key"))
	if err != nil {
		return err
	}

	serverCertBytes, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverKey.PublicKey, caPrivKey)
	if err != nil {
		return fmt.Errorf("failed to create server cert: %w", err)
	}

	if err := writeCert(c.serverCRT, serverCertBytes); err != nil {
		return err
	}
	if err := writeKey(c.serverKEY, serverKey); err != nil {
		return err
	}

	fmt.Println("✅ Certificates generated and saved:")
	fmt.Println(" -", c.clientCRT)
	fmt.Println(" -", c.serverCRT)
	fmt.Println(" -", c.serverKEY)

	return nil
}

func (c *certificate) Files() (string, string, string) {
	return c.clientCRT, c.serverCRT, c.serverKEY
}

func writeCert(path string, certBytes []byte) error {
	certOut, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certOut.Close()
	return pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
}

func writeKey(path string, key *rsa.PrivateKey) error {
	keyOut, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyOut.Close()
	return pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func readCert(path string) (*x509.Certificate, error) {
	certBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(certBytes)
	return x509.ParseCertificate(block.Bytes)
}

func readKey(path string) (*rsa.PrivateKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyBytes)
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
