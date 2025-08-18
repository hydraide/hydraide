package certificate

import (
	"bufio"
	"fmt"
	"strings"
)

type Prompts interface {
	Start(reader *bufio.Reader)
	GetCN() string
	GetDNS() []string
	GetIP() []string
}

type prompts struct {
	CN  string
	DNS []string
	IP  []string
}

func NewPrompts() Prompts {

	return &prompts{
		CN:  "",
		DNS: []string{},
		IP:  []string{},
	}

}

func (p *prompts) Start(reader *bufio.Reader) {

	// Certificate CN – default = localhost
	fmt.Println("🌐 TLS Certificate Setup")
	fmt.Println("🔖 Common Name (CN) is the main name assigned to the certificate.")
	fmt.Println("It usually identifies your company or internal system.")
	fmt.Print("CN (e.g. yourcompany, api.hydraide.local) [default: hydraide]: ")
	cnInput, _ := reader.ReadString('\n')
	p.CN = strings.TrimSpace(cnInput)
	if p.CN == "" {
		p.CN = "hydraide"
	}

	// Add localhost
	p.DNS = append(p.DNS, "localhost")
	p.IP = append(p.IP, "127.0.0.1")

	// Additional IP addresses
	fmt.Println("\n🌐 Add additional IP addresses to the certificate?")
	fmt.Println("By default, '127.0.0.1' is included for localhost access.")
	fmt.Println()
	fmt.Println("Now, list any other IP addresses where clients will access the HydrAIDE server.")
	fmt.Println("For example, if the HydrAIDE container is reachable at 192.168.106.100:4900, include that IP.")
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

	fmt.Println("\n🌐 Will clients connect via a domain name (FQDN)?")
	fmt.Println("This includes public domains (e.g. api.example.com) or internal DNS (e.g. hydraide.lan).")
	fmt.Println("To ensure secure TLS connections, you must list any domains that clients will use.")
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

func (p *prompts) GetCN() string {
	return p.CN
}

func (p *prompts) GetDNS() []string {
	return p.DNS
}

func (p *prompts) GetIP() []string {
	return p.IP
}
