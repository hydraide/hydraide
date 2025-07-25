package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

type CertConfig struct {
	CN  string
	DNS []string
	IP  []string
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Run the quick install wizard",
	Run: func(cmd *cobra.Command, args []string) {

		reader := bufio.NewReader(os.Stdin)

		fmt.Println("🚀 Starting HydrAIDE install wizard...\n")

		var cert CertConfig

		// Certificate CN – default = localhost
		fmt.Println("🌐 TLS Certificate Setup")
		fmt.Println("🔖 Common Name (CN) is the main name assigned to the certificate.")
		fmt.Println("It usually identifies your company or internal system.")
		fmt.Print("CN (e.g. yourcompany, api.hydraide.local) (default: hydraide): ")
		cnInput, _ := reader.ReadString('\n')
		cert.CN = strings.TrimSpace(cnInput)
		if cert.CN == "" {
			cert.CN = "hydraide"
		}

		// localhost hozzáadása
		cert.DNS = append(cert.DNS, "localhost")
		cert.IP = append(cert.IP, "127.0.0.1")

		// IP-k:belső s külső címek
		fmt.Println("\n🌐 Add additional IP addresses to the certificate?")
		fmt.Println("By default, '127.0.0.1' is included for localhost access.")
		fmt.Println()
		fmt.Println("Now, list any other IP addresses where clients will access the HydrAIDE server.")
		fmt.Println("For example, if the HydrAIDE container is reachable at 192.168.106.100:4900, include that IP.")
		fmt.Println("These IPs must match the address used in the TLS connection, or it will fail.")
		fmt.Print("Do you want to add other IPs besides 127.0.0.1? (y/n): ")

		ans, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			fmt.Print("Enter IPs (comma-separated, e.g. 192.168.1.5,10.0.0.12): ")
			ipInput, _ := reader.ReadString('\n')
			ips := strings.Split(strings.TrimSpace(ipInput), ",")
			for _, ip := range ips {
				ip = strings.TrimSpace(ip)
				if ip != "" {
					cert.IP = append(cert.IP, ip)
				}
			}
		}

		fmt.Println("\n🌐 Will clients connect via a domain name (FQDN)?")
		fmt.Println("This includes public domains (e.g. api.example.com) or internal DNS (e.g. hydraide.lan).")
		fmt.Println("To ensure secure TLS connections, you must list any domains that clients will use.")
		fmt.Print("Add domain names to the certificate? (y/n): ")
		ans, _ = reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			fmt.Print("Enter domain names (comma-separated, e.g. api.example.com,hydraide.local): ")
			dnsInput, _ := reader.ReadString('\n')
			domains := strings.Split(strings.TrimSpace(dnsInput), ",")
			for _, d := range domains {
				d = strings.TrimSpace(d)
				if d != "" {
					cert.DNS = append(cert.DNS, d)
				}
			}
		}

		fmt.Println("\n🔌 Port Configuration")
		fmt.Println("This is the external port on your host machine that will map to the HydrAIDE container.")
		fmt.Println("Clients will use this port to communicate with the HydrAIDE server.")
		fmt.Println("Make sure the port is available, open on firewalls, and not already in use.")
		fmt.Print("Which port should HydrAIDE listen on? (default: 4900): ")

		port, _ := reader.ReadString('\n')
		port = strings.TrimSpace(port)
		if port == "" {
			port = "4900"
		}

		fmt.Println("\n📁 Base Path for HydrAIDE")
		fmt.Println("This is the main directory where HydrAIDE will store its core files:")
		fmt.Println("  • TLS certificates")
		fmt.Println("  • Swamp Data Files")
		fmt.Println("  • Configuration and settings")
		fmt.Println()
		fmt.Println("Make sure the installer has permission to create folders and write files under this path.")
		fmt.Println("If the path does not exist, it will be created automatically.")

		fmt.Print("Base path (default: /mnt/hydraide): ")
		basePath, _ := reader.ReadString('\n')
		basePath = strings.TrimSpace(basePath)
		if basePath == "" {
			basePath = "/mnt/hydraide"
		}

		// todo: get information for the .env file

		// configuration summary
		fmt.Println("\n🔧 Configuration Summary:")
		fmt.Println("  • CN:         ", cert.CN)

		if len(cert.DNS) > 0 {
			fmt.Println("  • DNS SANs:   ", strings.Join(cert.DNS, ", "))
		} else {
			fmt.Println("  • DNS SANs:   [none ❗]")
		}

		if len(cert.IP) > 0 {
			fmt.Println("  • IP SANs:    ", strings.Join(cert.IP, ", "))
		} else {
			fmt.Println("  • IP SANs:    [none ❗]")
		}

		fmt.Println("  • Port:       ", port)
		fmt.Println("  • Base Path:  ", basePath)

		// todo: print the .env file content

		fmt.Print("\n✅ Proceed with installation? (y/n): ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.ToLower(strings.TrimSpace(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println("🚫 Installation cancelled.")
			return
		}

		fmt.Println("\n✅ Starting installation...")

		// todo: start the instance installation process

		// - todo: create the necessary directories
		// - todo: generate the TLS certificate
		// - todo: copy the server and client TLS certificate to the certificate directory
		// - todo: create the .env file (based on the .env_sample) to base path and fill in the values
		// - todo: download the latest binary (or the tagged one) from the github releases
		// - todo: create a service file based on the user's operating system
		// - todo: start the service

	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func defaultInstallPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "HydrAIDE", "bin")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "HydrAIDE", "bin")
	default:
		return filepath.Join(home, ".hydraide", "bin")
	}
}
