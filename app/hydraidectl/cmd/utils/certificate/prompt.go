package certificate

import (
	"bufio"
	"fmt"
	"strings"
)

// Prompts defines the interface for collecting TLS certificate parameters
// interactively from the user.
//
// ✅ Use this when:
// - You need to generate a self-signed TLS certificate for HydrAIDE
// - You want to prompt for Common Name (CN), IP addresses, and domain names
//
// Typical flow:
//  1. Call NewPrompts() to create an empty prompt handler
//  2. Call Start(reader) with a *bufio.Reader (e.g. os.Stdin)
//  3. Retrieve the collected values via GetCN(), GetIP(), GetDNS()
type Prompts interface {
	Start(reader *bufio.Reader)
	GetCN() string
	GetDNS() []string
	GetIP() []string
}

// prompts is the concrete implementation of Prompts.
// It stores the Common Name (CN), DNS names, and IP addresses
// that will be embedded into the certificate’s Subject Alternative Name (SAN).
type prompts struct {
	CN  string
	DNS []string
	IP  []string
}

// NewPrompts creates and returns a new Prompts instance
// with default, empty values.
//
// Example:
//
//	p := certificate.NewPrompts()
//	p.Start(bufio.NewReader(os.Stdin))
func NewPrompts() Prompts {
	return &prompts{
		CN:  "",
		DNS: []string{},
		IP:  []string{},
	}
}

// Start launches the interactive prompt sequence to collect
// certificate configuration details from the user.
//
// 🧭 Steps:
//  1. Ask for the Common Name (CN) → defaults to "hydraide"
//  2. Automatically include "localhost" (DNS) and "127.0.0.1" (IP)
//  3. Optionally add more IP addresses
//  4. Optionally add fully qualified domain names (FQDNs)
//
// These inputs determine the certificate’s Subject Alternative Names (SANs).
// TLS clients must connect using one of these values for validation to succeed.
func (p *prompts) Start(reader *bufio.Reader) {
	// Common Name (CN)
	fmt.Println("🌐 TLS Certificate Setup")
	fmt.Println("🔖 Common Name (CN) is the main name assigned to the certificate.")
	fmt.Println("It usually identifies your company or internal system.")
	fmt.Print("CN (e.g. yourcompany, api.hydraide.local) [default: hydraide]: ")
	cnInput, _ := reader.ReadString('\n')
	p.CN = strings.TrimSpace(cnInput)
	if p.CN == "" {
		p.CN = "hydraide"
	}

	// Always include localhost
	p.DNS = append(p.DNS, "localhost")
	p.IP = append(p.IP, "127.0.0.1")

	// Additional IP addresses
	fmt.Println("\n🌐 Add additional IP addresses to the certificate?")
	fmt.Println("By default, '127.0.0.1' is included for localhost access.")
	fmt.Println("Now, list any other IP addresses where clients will access the HydrAIDE server.")
	fmt.Println("These IPs must match the address used in the TLS connection, or it will fail.")
	fmt.Print("Do you want to add other IPs besides 127.0.0.1? (y/n) [default: n]: ")

	ans, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(ans)) == "y" {
		fmt.Print("Enter IPs (comma-separated, e.g. 192.168.1.5,10.0.0.12): ")
		ipInput, _ := reader.ReadString('\n')
		ips := strings.Split(strings.TrimSpace(ipInput), ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				p.IP = append(p.IP, ip)
			}
		}
	}

	// Domain names (FQDNs)
	fmt.Println("\n🌐 Will clients connect via a domain name (FQDN)?")
	fmt.Println("This includes public domains (e.g. api.example.com) or internal DNS (e.g. hydraide.lan).")
	fmt.Print("Add domain names to the certificate? (y/n) [default: n]: ")
	ans, _ = reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(ans)) == "y" {
		fmt.Print("Enter domain names (comma-separated, e.g. api.example.com,hydraide.local): ")
		dnsInput, _ := reader.ReadString('\n')
		domains := strings.Split(strings.TrimSpace(dnsInput), ",")
		for _, d := range domains {
			d = strings.TrimSpace(d)
			if d != "" {
				p.DNS = append(p.DNS, d)
			}
		}
	}
}

// GetCN returns the Common Name (CN) provided by the user or the default.
func (p *prompts) GetCN() string {
	return p.CN
}

// GetDNS returns all DNS names collected for SAN.
func (p *prompts) GetDNS() []string {
	return p.DNS
}

// GetIP returns all IP addresses collected for SAN.
func (p *prompts) GetIP() []string {
	return p.IP
}
