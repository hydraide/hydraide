package cmd

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/certificate"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/elevation"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/env"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancehealth"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/portfinder"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/validator"
	"github.com/spf13/cobra"
)

var editInstance string

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit the configuration of an existing HydrAIDE instance",
	Long: `Opens a section-based editor for an existing HydrAIDE instance:
ports, logging, gRPC settings, TLS SANs, systemd unit. Settings are validated
the same way as during 'hydraidectl init'. After saving, the instance is
restarted and a health check is performed.

Binary version changes are NOT handled here — use 'hydraidectl upgrade'.`,
	Run: runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)
	editCmd.Flags().StringVarP(&editInstance, "instance", "i", "",
		"Instance name to edit")
	if err := editCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Println("Error marking 'instance' flag as required:", err)
		os.Exit(1)
	}
}

// editChanges tracks which sections were modified so we know what to write,
// regenerate, or restart at the end.
type editChanges struct {
	envChanged     bool
	certsChanged   bool
	serviceMissing bool
	serviceFixed   bool
}

func runEdit(cmd *cobra.Command, args []string) {
	if !elevation.IsElevated() {
		fmt.Println(elevation.Hint(editInstance))
		os.Exit(3)
	}

	reader := bufio.NewReader(os.Stdin)
	fs := filesystem.New()
	ctx := context.Background()
	v := validator.New()

	bm, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Failed to load metadata store: %v\n", err)
		os.Exit(1)
	}
	meta, err := bm.GetInstance(editInstance)
	if err != nil {
		fmt.Printf("❌ Instance %q not found. Run 'hydraidectl list' to see registered instances.\n", editInstance)
		os.Exit(1)
	}

	e := env.New(fs, meta.BasePath)
	if !e.IsExists(ctx) {
		fmt.Printf("❌ No .env file found at %s — instance is corrupt; consider 'destroy' + 'init'.\n", e.GetEnvPath())
		os.Exit(1)
	}
	settings, err := e.Load(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to read .env: %v\n", err)
		os.Exit(1)
	}
	settings.HydrAIDEBasePath = meta.BasePath

	sp := servicehelper.New()
	unitExists, err := sp.ServiceExists(editInstance)
	if err != nil {
		fmt.Printf("⚠️  Could not check systemd unit: %v\n", err)
	}

	changes := &editChanges{serviceMissing: !unitExists}

	// Read the current SANs from the server certificate so the editor shows
	// what is actually deployed. Falls back to the install-time defaults if
	// the cert cannot be parsed (e.g. missing or unreadable).
	cn, dns, ip := readCurrentSANs(meta.BasePath)

	for {
		printEditMenu(settings, cn, dns, ip, changes)
		fmt.Print("> ")
		choice, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1":
			if editPorts(reader, ctx, fs, v, settings) {
				changes.envChanged = true
			}
		case "2":
			if editLogging(reader, ctx, v, settings) {
				changes.envChanged = true
			}
		case "3":
			if editGRPC(reader, ctx, v, settings) {
				changes.envChanged = true
			}
		case "4":
			newCN, newDNS, newIP, ok := editCertSANs(reader, cn, dns, ip)
			if ok {
				cn, dns, ip = newCN, newDNS, newIP
				changes.certsChanged = true
			}
		case "5":
			if changes.serviceMissing {
				if reinstallService(sp, editInstance, meta.BasePath) {
					changes.serviceFixed = true
					changes.serviceMissing = false
				}
			} else {
				fmt.Println("ℹ️  systemd unit is present; nothing to reinstall.")
			}
		case "s":
			if !changes.envChanged && !changes.certsChanged && !changes.serviceFixed {
				fmt.Println("ℹ️  Nothing changed. Exiting.")
				return
			}
			applyEdits(ctx, fs, e, settings, cn, dns, ip, changes, sp, meta.BasePath, reader)
			return
		case "q":
			if changes.envChanged || changes.certsChanged {
				fmt.Print("⚠️  Discard unsaved changes? (y/n): ")
				ans, _ := reader.ReadString('\n')
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
					continue
				}
			}
			fmt.Println("🚫 Edit cancelled.")
			return
		default:
			fmt.Println("❌ Unknown option.")
		}
	}
}

func printEditMenu(s *env.Settings, cn string, dns, ip []string, c *editChanges) {
	mark := func(b bool) string {
		if b {
			return " *modified*"
		}
		return ""
	}
	unitTag := "[unit OK]"
	if c.serviceMissing {
		unitTag = "[MISSING — reinstall recommended]"
	} else if c.serviceFixed {
		unitTag = "[reinstalled]"
	}

	fmt.Println("\n──────────────────────────────────────────────")
	fmt.Printf(" hydraidectl edit — instance: %s\n", editInstance)
	fmt.Println("──────────────────────────────────────────────")
	fmt.Printf("  [1] Ports          gRPC=%s, health=%s%s\n",
		s.HydrAIDEGRPCPort, s.HydrAIDEHealthCheckPort, mark(c.envChanged))
	fmt.Printf("  [2] Logging        level=%s, graylog=%v, resource=%v\n",
		s.LogLevel, s.GraylogEnabled, s.SystemResourceLogging)
	fmt.Printf("  [3] gRPC           maxSize=%d, errorLog=%v\n",
		s.GRPCMaxMessageSize, s.GRPCServerErrorLogging)
	fmt.Printf("  [4] TLS SANs       CN=%s, DNS=[%s], IP=[%s]%s\n",
		cn, strings.Join(dns, ","), strings.Join(ip, ","), mark(c.certsChanged))
	fmt.Printf("  [5] systemd unit   %s\n", unitTag)
	fmt.Println("  [s] Save and restart")
	fmt.Println("  [q] Quit without saving")
}

func editPorts(reader *bufio.Reader, ctx context.Context, fs filesystem.FileSystem, v validator.Validator, s *env.Settings) bool {
	curGRPC, _ := strconv.Atoi(s.HydrAIDEGRPCPort)
	fmt.Printf("Current gRPC port: %d (health = %d)\n", curGRPC, curGRPC+1)
	fmt.Print("New gRPC port (enter to keep): ")
	in, _ := reader.ReadString('\n')
	in = strings.TrimSpace(in)
	if in == "" {
		return false
	}
	valid, err := v.ValidatePort(ctx, in)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return false
	}
	p, _ := strconv.Atoi(valid)
	if p == curGRPC {
		return false
	}
	if !portfinder.IsPortFree(p) || !portfinder.IsPortFree(p+1) {
		fmt.Printf("❌ Port %d or %d is in use.\n", p, p+1)
		return false
	}
	s.HydrAIDEGRPCPort = strconv.Itoa(p)
	s.HydrAIDEHealthCheckPort = strconv.Itoa(p + 1)
	fmt.Printf("✅ Ports set to %d / %d\n", p, p+1)
	return true
}

func editLogging(reader *bufio.Reader, ctx context.Context, v validator.Validator, s *env.Settings) bool {
	changed := false

	fmt.Printf("Current log level: %s\n", s.LogLevel)
	fmt.Print("New log level (debug|info|warn|error, enter to keep): ")
	in, _ := reader.ReadString('\n')
	in = strings.TrimSpace(in)
	if in != "" {
		level, err := v.ValidateLoglevel(ctx, in)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
		} else if level != s.LogLevel {
			s.LogLevel = level
			changed = true
		}
	}

	fmt.Printf("System resource logging is currently %v\n", s.SystemResourceLogging)
	fmt.Print("Toggle? (y = flip / n = keep) [default: n]: ")
	res, _ := reader.ReadString('\n')
	if isYes(res) {
		s.SystemResourceLogging = !s.SystemResourceLogging
		changed = true
	}

	fmt.Printf("Graylog is currently %v\n", s.GraylogEnabled)
	fmt.Print("Toggle? (y = flip / n = keep) [default: n]: ")
	gr, _ := reader.ReadString('\n')
	if isYes(gr) {
		s.GraylogEnabled = !s.GraylogEnabled
		changed = true
	}
	if s.GraylogEnabled {
		fmt.Printf("Graylog server [current: %s]: ", s.GraylogServer)
		srv, _ := reader.ReadString('\n')
		srv = strings.TrimSpace(srv)
		if srv != "" && srv != s.GraylogServer {
			s.GraylogServer = srv
			changed = true
		}
		fmt.Printf("Graylog service name [current: %s]: ", s.GraylogServiceName)
		svc, _ := reader.ReadString('\n')
		svc = strings.TrimSpace(svc)
		if svc != "" && svc != s.GraylogServiceName {
			s.GraylogServiceName = svc
			changed = true
		}
	}
	return changed
}

func editGRPC(reader *bufio.Reader, ctx context.Context, v validator.Validator, s *env.Settings) bool {
	changed := false
	fmt.Printf("Current max message size: %s\n", v.FormatSize(ctx, s.GRPCMaxMessageSize))
	fmt.Print("New size (e.g. 100MB, enter to keep): ")
	in, _ := reader.ReadString('\n')
	in = strings.TrimSpace(in)
	if in != "" {
		size, err := v.ParseMessageSize(ctx, in)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
		} else if size != s.GRPCMaxMessageSize {
			s.GRPCMaxMessageSize = size
			changed = true
		}
	}

	fmt.Printf("gRPC error logging is currently %v\n", s.GRPCServerErrorLogging)
	fmt.Print("Toggle? (y = flip / n = keep) [default: n]: ")
	gerr, _ := reader.ReadString('\n')
	if isYes(gerr) {
		s.GRPCServerErrorLogging = !s.GRPCServerErrorLogging
		changed = true
	}
	return changed
}

// editCertSANs collects a fresh CN / DNS / IP set. Because the existing cert
// is regenerated from these inputs (not amended), the prompt asks for the
// full SAN list rather than additions.
func editCertSANs(reader *bufio.Reader, curCN string, curDNS, curIP []string) (string, []string, []string, bool) {
	fmt.Println("\n⚠️  Editing TLS SANs regenerates the certificate.")
	fmt.Println("   All clients must replace ca.crt / client.crt / client.key after restart.")
	fmt.Print("Continue? (y/n) [default: n]: ")
	ans, _ := reader.ReadString('\n')
	if !isYes(ans) {
		return curCN, curDNS, curIP, false
	}

	fmt.Printf("Common Name [current: %s]: ", curCN)
	cnIn, _ := reader.ReadString('\n')
	cn := strings.TrimSpace(cnIn)
	if cn == "" {
		cn = curCN
	}

	fmt.Printf("DNS SANs comma-separated [current: %s]: ", strings.Join(curDNS, ","))
	dnsIn, _ := reader.ReadString('\n')
	dns := splitCSV(dnsIn, curDNS)

	fmt.Printf("IP SANs comma-separated [current: %s]: ", strings.Join(curIP, ","))
	ipIn, _ := reader.ReadString('\n')
	ip := splitCSV(ipIn, curIP)

	fmt.Print("\nFinal: typing 'rotate' regenerates the certificate now: ")
	conf, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(conf)) != "rotate" {
		fmt.Println("🚫 Certificate rotation cancelled.")
		return curCN, curDNS, curIP, false
	}
	return cn, dns, ip, true
}

func splitCSV(in string, fallback []string) []string {
	in = strings.TrimSpace(in)
	if in == "" {
		return fallback
	}
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func reinstallService(sp servicehelper.ServiceManager, instanceName, basePath string) bool {
	fmt.Println("\n🛠️  Reinstalling systemd unit (config + .env only, binary untouched)...")
	if err := sp.GenerateServiceFile(instanceName, basePath); err != nil {
		fmt.Printf("❌ Generate unit: %v\n", err)
		return false
	}
	if err := sp.EnableAndStartService(instanceName, basePath); err != nil {
		fmt.Printf("❌ Enable/start: %v\n", err)
		return false
	}
	fmt.Println("✅ systemd unit reinstalled and started.")
	return true
}

func applyEdits(
	ctx context.Context,
	fs filesystem.FileSystem,
	e env.Env,
	settings *env.Settings,
	cn string, dns, ip []string,
	c *editChanges,
	sp servicehelper.ServiceManager,
	basePath string,
	reader *bufio.Reader,
) {
	if c.envChanged {
		if err := e.Set(ctx, settings); err != nil {
			fmt.Printf("❌ Failed to write .env: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ .env updated")
	}

	if c.certsChanged {
		if err := regenerateCerts(ctx, fs, cn, dns, ip, basePath); err != nil {
			fmt.Printf("❌ Cert regeneration failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ TLS certificates regenerated in", filepath.Join(basePath, "certificate"))
	}

	if !c.serviceFixed {
		fmt.Println()
		fmt.Println("⚠️  Restart will stop and re-start the service.")
		fmt.Println("   Stop ALL client applications that hold open connections to this instance")
		fmt.Println("   first — HydrAIDE protects in-flight data and will not shut down gracefully")
		fmt.Println("   while clients are still connected (the stop phase will hang).")
		if !confirm(reader, "   Have you stopped all clients? (y/n) [default: n]: ", false) {
			fmt.Println("🚫 Restart skipped. Run 'sudo hydraidectl restart -i " + editInstance + "' manually after stopping your clients.")
			fmt.Println("   Configuration changes are saved and will take effect on the next restart.")
			return
		}

		fmt.Println("\n🔄 Restarting service...")
		runner := instancerunner.NewInstanceController()
		if runner == nil {
			fmt.Println("⚠️  Could not initialize instance controller; restart manually with: sudo hydraidectl restart -i " + editInstance)
		} else if err := runner.RestartInstance(ctx, editInstance); err != nil {
			fmt.Printf("⚠️  Restart returned: %v\n", err)
		}
	}

	fmt.Print("⏳ Waiting for instance to become healthy")
	ih := instancehealth.NewInstanceHealth()
	for i := 0; i < 30; i++ {
		fmt.Print(".")
		st := ih.GetHealthStatus(ctx, editInstance)
		if st.Status == "healthy" {
			fmt.Println(" ✅")
			break
		}
		if i == 29 {
			fmt.Println(" ❌")
			fmt.Println("⚠️  Instance is not healthy yet. Check: journalctl -u hydraserver-" + editInstance + " -n 100")
		}
	}

	fmt.Println("\n🎉 Edit complete.")
	if c.certsChanged {
		fmt.Println("👉 Re-distribute the new ca.crt / client.crt / client.key to all clients.")
	}
}

// readCurrentSANs parses <basePath>/certificate/server.crt and returns the
// CN, DNS SANs and IP SANs that are actually in effect on this instance.
// On any error it returns the install-time defaults so the editor stays
// usable for instances whose certs are missing or were generated by tooling
// that did not place server.crt in the standard location.
func readCurrentSANs(basePath string) (string, []string, []string) {
	defaults := func() (string, []string, []string) {
		return "hydraide", []string{"localhost"}, []string{"127.0.0.1"}
	}
	pemBytes, err := os.ReadFile(filepath.Join(basePath, "certificate", "server.crt"))
	if err != nil {
		return defaults()
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return defaults()
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return defaults()
	}
	ips := make([]string, 0, len(crt.IPAddresses))
	for _, ip := range crt.IPAddresses {
		ips = append(ips, ip.String())
	}
	dns := append([]string(nil), crt.DNSNames...)
	if len(dns) == 0 {
		dns = []string{"localhost"}
	}
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	return crt.Subject.CommonName, dns, ips
}

func regenerateCerts(ctx context.Context, fs filesystem.FileSystem, cn string, dns, ip []string, basePath string) error {
	cp := certificate.NewPrompts()
	cp.SetSANs(cn, dns, ip)
	if err := cp.GenerateCert(); err != nil {
		return fmt.Errorf("generate cert: %w", err)
	}

	for _, file := range cp.GetCertificateFiles() {
		dest := filepath.Join(basePath, "certificate", filepath.Base(file))
		if err := fs.MoveFile(ctx, file, dest); err != nil {
			return fmt.Errorf("move %s: %w", file, err)
		}
	}
	return nil
}
