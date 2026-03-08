package explore

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hydraide/hydraide/app/server/explorer"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const confirmCodeLen = 4
const deleteBatchSize = 500

// generateCode creates a random 4-character alphanumeric confirmation code.
func generateCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1 to avoid confusion
	code := make([]byte, confirmCodeLen)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		code[i] = charset[n.Int64()]
	}
	return string(code)
}

// initiateDelete starts the delete confirmation flow based on the current view level.
func (m *Model) initiateDelete() {
	if m.instanceName == "" {
		m.deleteError = "Delete requires --instance flag (server must be running)"
		return
	}

	switch m.level {
	case levelSanctuaries:
		items := m.filteredSanctuaries()
		if m.cursor >= len(items) {
			return
		}
		sanctuary := items[m.cursor]
		m.deleteTargetName = fmt.Sprintf("Sanctuary: %s", sanctuary.Name)
		m.deleteSwampList = m.collectSwamps(sanctuary.Name, "", "")
		m.deleteMode = levelSanctuaries

	case levelRealms:
		items := m.filteredRealms()
		if m.cursor >= len(items) {
			return
		}
		realm := items[m.cursor]
		m.deleteTargetName = fmt.Sprintf("Realm: %s/%s", m.selectedSanctuary, realm.Name)
		m.deleteSwampList = m.collectSwamps(m.selectedSanctuary, realm.Name, "")
		m.deleteMode = levelRealms

	case levelSwamps:
		items := m.filteredSwamps()
		if m.cursor >= len(items) {
			return
		}
		sw := items[m.cursor]
		m.deleteTargetName = fmt.Sprintf("Swamp: %s/%s/%s", sw.Sanctuary, sw.Realm, sw.Swamp)
		m.deleteSwampList = []*explorer.SwampDetail{sw}
		m.deleteMode = levelSwamps

	case levelDetail:
		if m.detail == nil {
			return
		}
		m.deleteTargetName = fmt.Sprintf("Swamp: %s/%s/%s", m.detail.Sanctuary, m.detail.Realm, m.detail.Swamp)
		m.deleteSwampList = []*explorer.SwampDetail{m.detail}
		m.deleteMode = levelDetail

	default:
		return
	}

	if len(m.deleteSwampList) == 0 {
		m.deleteError = "No swamps found to delete"
		return
	}

	m.deleteConfirm = 1
	m.deleteCode = generateCode()
	m.deleteInput = ""
}

// collectSwamps gathers all swamp details matching the given scope.
func (m *Model) collectSwamps(sanctuary, realm, swamp string) []*explorer.SwampDetail {
	if swamp != "" {
		detail, err := m.explorer.GetSwampDetail(sanctuary, realm, swamp)
		if err != nil {
			return nil
		}
		return []*explorer.SwampDetail{detail}
	}

	filter := &explorer.SwampFilter{
		Sanctuary: sanctuary,
		Realm:     realm,
		Limit:     1000000,
	}
	result := m.explorer.ListSwamps(filter)
	return result.Swamps
}

// handleDeleteConfirmInput processes key input during confirmation dialogs.
func (m *Model) handleDeleteConfirmInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.deleteConfirm = 0
		m.deleteInput = ""
		return m, nil

	case "ctrl+c":
		return m, tea.Quit

	case "enter":
		if m.deleteInput == m.deleteCode {
			if m.deleteConfirm == 1 {
				// Move to second confirmation
				m.deleteConfirm = 2
				m.deleteCode = generateCode()
				m.deleteInput = ""
			} else {
				// Start deletion
				m.deleteConfirm = 0
				m.deleting = true
				m.deleteTotal = int64(len(m.deleteSwampList))
				m.deleteProgress = deleteProgressMsg{}
				return m, tea.Batch(m.startDelete(), m.deleteTick())
			}
		}
		return m, nil

	case "backspace":
		if len(m.deleteInput) > 0 {
			m.deleteInput = m.deleteInput[:len(m.deleteInput)-1]
		}
		return m, nil

	default:
		ch := msg.String()
		if len(ch) == 1 && len(m.deleteInput) < confirmCodeLen {
			m.deleteInput += strings.ToUpper(ch)
		}
		return m, nil
	}
}

// deleteTick sends periodic progress updates during deletion.
func (m Model) deleteTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
		return m.deleteProgress
	})
}

// startDelete initiates the bulk destroy operation via gRPC streaming.
func (m *Model) startDelete() tea.Cmd {
	swamps := m.deleteSwampList
	basePath := m.basePath

	return func() tea.Msg {
		start := time.Now()

		conn, err := connectToServer(basePath)
		if err != nil {
			return deleteDoneMsg{lastError: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer conn.Close()

		client := hydrapb.NewHydraideServiceClient(conn)

		stream, err := client.DestroyBulk(context.Background())
		if err != nil {
			return deleteDoneMsg{lastError: fmt.Sprintf("Stream open failed: %v", err)}
		}

		// Send targets in batches
		for i := 0; i < len(swamps); i += deleteBatchSize {
			end := i + deleteBatchSize
			if end > len(swamps) {
				end = len(swamps)
			}

			batch := &hydrapb.DestroyBulkRequest{}
			for _, sw := range swamps[i:end] {
				islandID := parseIslandID(sw.IslandID)
				batch.Targets = append(batch.Targets, &hydrapb.DestroyBulkTarget{
					IslandID:  islandID,
					SwampName: fmt.Sprintf("%s/%s/%s", sw.Sanctuary, sw.Realm, sw.Swamp),
				})
			}

			if err := stream.Send(batch); err != nil {
				return deleteDoneMsg{lastError: fmt.Sprintf("Send failed: %v", err)}
			}
		}

		// Close send side
		if err := stream.CloseSend(); err != nil {
			return deleteDoneMsg{lastError: fmt.Sprintf("CloseSend failed: %v", err)}
		}

		// Read progress responses until done
		var lastResp *hydrapb.DestroyBulkResponse
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			lastResp = resp
			if resp.Done {
				break
			}
		}

		if lastResp != nil {
			return deleteDoneMsg{
				destroyed: lastResp.Destroyed,
				failed:    lastResp.Failed,
				duration:  time.Since(start),
				lastError: lastResp.LastError,
			}
		}

		return deleteDoneMsg{
			duration:  time.Since(start),
			lastError: "No response received from server",
		}
	}
}

// parseIslandID converts the string island ID from explorer to uint64.
func parseIslandID(id string) uint64 {
	var n uint64
	fmt.Sscanf(id, "%d", &n)
	return n
}

// connectToServer establishes an mTLS gRPC connection to the HydrAIDE server.
func connectToServer(basePath string) (*grpc.ClientConn, error) {
	certsPath := filepath.Join(basePath, "certificate")
	certFile := filepath.Join(certsPath, "client.crt")
	keyFile := filepath.Join(certsPath, "client.key")
	caFile := filepath.Join(certsPath, "ca.crt")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
	}

	creds := credentials.NewTLS(tlsConfig)

	// Read server port from .env
	serverAddr := resolveAddr(basePath)

	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpc connect: %w", err)
	}

	return conn, nil
}

// resolveAddr reads the server address from instance .env file.
func resolveAddr(basePath string) string {
	envPath := filepath.Join(basePath, ".env")
	port := "5554"
	if envData, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(envData), "\n") {
			if strings.HasPrefix(line, "HYDRAIDE_SERVER_PORT=") {
				port = strings.TrimPrefix(line, "HYDRAIDE_SERVER_PORT=")
				port = strings.TrimSpace(port)
				break
			}
		}
	}
	return fmt.Sprintf("localhost:%s", port)
}

// ── Delete View Renderers ───────────────────────────────────────────────────

func (m Model) renderDeleteConfirm() string {
	var b strings.Builder

	totalSize := int64(0)
	for _, sw := range m.deleteSwampList {
		totalSize += sw.FileSize
	}

	if m.deleteConfirm == 1 {
		b.WriteString("\n")
		b.WriteString("  " + deleteWarningStyle.Render(" ⚠  DELETE CONFIRMATION ") + "\n")
		b.WriteString("\n")
		b.WriteString("  " + rowStyle.Render("You are about to delete:") + "\n")
		b.WriteString("\n")
		b.WriteString("  " + detailLabelStyle.Render("Target:") + "  " + deleteTargetStyle.Render(m.deleteTargetName) + "\n")
		b.WriteString("  " + detailLabelStyle.Render("Swamps:") + "  " + valueStyle.Render(fmt.Sprintf("%d", len(m.deleteSwampList))) + "\n")
		b.WriteString("  " + detailLabelStyle.Render("Total size:") + "  " + valueStyle.Render(formatSize(totalSize)) + "\n")
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Type %s to confirm: %s",
			deleteCodeStyle.Render("["+m.deleteCode+"]"),
			deleteInputStyle.Render(m.deleteInput)+cursorStyle.Render("_")))
		b.WriteString("\n\n")
		b.WriteString("  " + labelStyle.Render("Press ESC to cancel"))
	} else {
		b.WriteString("\n")
		b.WriteString("  " + deleteDangerStyle.Render(" 🔴 FINAL WARNING — IRREVERSIBLE! ") + "\n")
		b.WriteString("\n")
		b.WriteString("  " + errorCountStyle.Render("THIS ACTION CANNOT BE UNDONE!") + "\n")
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s swamps will be permanently\n",
			deleteTargetStyle.Render(fmt.Sprintf("%d", len(m.deleteSwampList)))))
		b.WriteString("  destroyed from the live server.\n")
		b.WriteString("\n")
		b.WriteString("  " + detailLabelStyle.Render("Target:") + "  " + deleteTargetStyle.Render(m.deleteTargetName) + "\n")
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Type %s to PERMANENTLY DELETE: %s",
			deleteCodeStyle.Render("["+m.deleteCode+"]"),
			deleteInputStyle.Render(m.deleteInput)+cursorStyle.Render("_")))
		b.WriteString("\n\n")
		b.WriteString("  " + labelStyle.Render("Press ESC to cancel"))
	}

	return b.String()
}

func (m Model) renderDeleting() string {
	var b strings.Builder

	b.WriteString("\n")
	p := m.deleteProgress
	total := m.deleteTotal
	done := p.destroyed + p.failed

	// Progress bar
	barWidth := 30
	if m.width > 60 {
		barWidth = m.width/2 - 10
	}
	filled := 0
	if total > 0 {
		filled = int(done * int64(barWidth) / total)
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	pct := 0
	if total > 0 {
		pct = int(done * 100 / total)
	}

	b.WriteString(fmt.Sprintf("  Deleting... %d / %d swamps  [%s] %d%%\n",
		done, total, scanStyle.Render(bar), pct))

	if p.failed > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", errorCountStyle.Render(fmt.Sprintf("Errors: %d", p.failed))))
	}

	return b.String()
}

func (m Model) renderDeleteDone() string {
	var b strings.Builder

	b.WriteString("\n")
	r := m.deleteResult
	if r.lastError != "" && r.destroyed == 0 {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			errorCountStyle.Render("✗"),
			errorCountStyle.Render(fmt.Sprintf("Deletion failed: %s", r.lastError))))
	} else {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			scanStyle.Render("✓"),
			scanStyle.Render(fmt.Sprintf("Deletion complete: %d swamps destroyed in %s",
				r.destroyed, r.duration.Round(time.Millisecond)))))
		if r.failed > 0 {
			b.WriteString(fmt.Sprintf("  %s\n",
				errorCountStyle.Render(fmt.Sprintf("%d swamps failed (last error: %s)", r.failed, r.lastError))))
		}
	}
	b.WriteString("\n  " + labelStyle.Render("Press any key to continue"))

	return b.String()
}
