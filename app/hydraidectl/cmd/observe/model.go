package observe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Tab represents the current view tab
type Tab int

const (
	TabLive Tab = iota
	TabErrors
	TabStats
	TabLongRunning
)

// Model is the Bubbletea model for the observe TUI
type Model struct {
	conn       *grpc.ClientConn
	client     hydrapb.HydraideServiceClient
	cancelFunc context.CancelFunc
	connected  bool
	connError  string

	events      []Event
	selectedIdx int
	maxEvents   int
	paused      bool
	pauseBuffer []Event

	activeTab      Tab
	eventsViewport viewport.Model

	stats *hydrapb.TelemetryStatsResponse

	selectedError   *Event
	showErrorDetail bool

	errorsOnly  bool
	swampFilter string

	width  int
	height int

	showHelp bool

	serverAddr string
	certFile   string
	keyFile    string
	caFile     string
}

// Event represents a telemetry event for display
type Event struct {
	ID           string
	Timestamp    time.Time
	Method       string
	SwampName    string
	Keys         []string
	DurationUs   int64
	Success      bool
	ErrorCode    string
	ErrorMessage string
	ClientIP     string
	HasDetails   bool
}

// Messages
type tickMsg time.Time
type eventMsg Event
type statsMsg *hydrapb.TelemetryStatsResponse
type connectedMsg struct {
	conn   *grpc.ClientConn
	client hydrapb.HydraideServiceClient
}
type errorMsg struct{ err error }

type historyBatchMsg struct {
	events []*hydrapb.TelemetryEvent
}

type streamEventMsg struct {
	event *hydrapb.TelemetryEvent
}

type streamErrorMsg struct {
	err error
}

// NewModel creates a new observe model
func NewModel(serverAddr, certFile, keyFile, caFile string) Model {
	return Model{
		maxEvents:   500,
		events:      make([]Event, 0, 500),
		pauseBuffer: make([]Event, 0),
		activeTab:   TabLive,
		serverAddr:  serverAddr,
		certFile:    certFile,
		keyFile:     keyFile,
		caFile:      caFile,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.connect(),
		tea.EnterAltScreen,
	)
}

// connect establishes connection to the HydrAIDE server
func (m Model) connect() tea.Cmd {
	return func() tea.Msg {
		cert, err := tls.LoadX509KeyPair(m.certFile, m.keyFile)
		if err != nil {
			return errorMsg{fmt.Errorf("failed to load client certificate: %w", err)}
		}

		caCert, err := os.ReadFile(m.caFile)
		if err != nil {
			return errorMsg{fmt.Errorf("failed to read CA certificate: %w", err)}
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return errorMsg{fmt.Errorf("failed to parse CA certificate")}
		}

		hostOnly := strings.Split(m.serverAddr, ":")[0]

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
			ServerName:   hostOnly,
			MinVersion:   tls.VersionTLS13,
		}

		creds := credentials.NewTLS(tlsConfig)

		conn, err := grpc.NewClient(m.serverAddr, grpc.WithTransportCredentials(creds))
		if err != nil {
			return errorMsg{fmt.Errorf("failed to connect: %w", err)}
		}

		client := hydrapb.NewHydraideServiceClient(conn)
		return connectedMsg{conn: conn, client: client}
	}
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.eventsViewport.Width = msg.Width - 4
		m.eventsViewport.Height = msg.Height - 10

	case connectedMsg:
		m.connected = true
		m.conn = msg.conn
		m.client = msg.client
		cmds = append(cmds, m.subscribeToTelemetry())
		cmds = append(cmds, m.fetchStats())

	case errorMsg:
		m.connError = msg.err.Error()

	case eventMsg:
		event := Event(msg)
		if m.paused {
			m.pauseBuffer = append(m.pauseBuffer, event)
		} else {
			m.addEvent(event)
		}

	case statsMsg:
		m.stats = msg
		cmds = append(cmds, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}))

	case tickMsg:
		cmds = append(cmds, m.fetchStats())

	case historyBatchMsg:
		for _, protoEvent := range msg.events {
			event := Event{
				ID:           protoEvent.Id,
				Timestamp:    protoEvent.Timestamp.AsTime(),
				Method:       protoEvent.Method,
				SwampName:    protoEvent.SwampName,
				Keys:         protoEvent.Keys,
				DurationUs:   protoEvent.DurationUs,
				Success:      protoEvent.Success,
				ErrorCode:    protoEvent.ErrorCode,
				ErrorMessage: protoEvent.ErrorMessage,
				ClientIP:     protoEvent.ClientIp,
				HasDetails:   protoEvent.HasStackTrace,
			}
			m.addEvent(event)
		}
		cmds = append(cmds, m.startStreaming())

	case streamEventMsg:
		protoEvent := msg.event
		event := Event{
			ID:           protoEvent.Id,
			Timestamp:    protoEvent.Timestamp.AsTime(),
			Method:       protoEvent.Method,
			SwampName:    protoEvent.SwampName,
			Keys:         protoEvent.Keys,
			DurationUs:   protoEvent.DurationUs,
			Success:      protoEvent.Success,
			ErrorCode:    protoEvent.ErrorCode,
			ErrorMessage: protoEvent.ErrorMessage,
			ClientIP:     protoEvent.ClientIp,
			HasDetails:   protoEvent.HasStackTrace,
		}
		if m.paused {
			m.pauseBuffer = append(m.pauseBuffer, event)
		} else {
			m.addEvent(event)
		}
		cmds = append(cmds, m.startStreaming())

	case streamErrorMsg:
		m.connError = fmt.Sprintf("Stream error: %v", msg.err)
	}

	return m, tea.Batch(cmds...)
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		if m.conn != nil {
			m.conn.Close()
		}
		return m, tea.Quit

	case "?", "h":
		m.showHelp = !m.showHelp
		return m, nil

	case "1":
		m.activeTab = TabLive
		return m, nil

	case "2":
		m.activeTab = TabErrors
		return m, nil

	case "3":
		m.activeTab = TabStats
		return m, nil

	case "4":
		m.activeTab = TabLongRunning
		return m, nil

	case "p":
		m.paused = !m.paused
		if !m.paused && len(m.pauseBuffer) > 0 {
			for _, e := range m.pauseBuffer {
				m.addEvent(e)
			}
			m.pauseBuffer = m.pauseBuffer[:0]
		}
		return m, nil

	case "c":
		m.events = m.events[:0]
		m.selectedIdx = 0
		return m, nil

	case "e":
		m.errorsOnly = !m.errorsOnly
		return m, nil

	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "down", "j":
		if m.selectedIdx < len(m.events)-1 {
			m.selectedIdx++
		}
	case "enter":
		if m.selectedIdx >= 0 && m.selectedIdx < len(m.events) {
			m.selectedError = &m.events[m.selectedIdx]
			m.showErrorDetail = true
		}
	case "esc":
		m.showErrorDetail = false
		m.showHelp = false
		m.selectedError = nil
	}

	return m, nil
}

// addEvent adds an event to the list
func (m *Model) addEvent(event Event) {
	if m.errorsOnly && event.Success {
		return
	}

	m.events = append(m.events, event)

	if len(m.events) > m.maxEvents {
		m.events = m.events[1:]
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	}

	if !m.paused {
		m.selectedIdx = len(m.events) - 1
	}
}

// subscribeToTelemetry starts the telemetry subscription
func (m Model) subscribeToTelemetry() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}

		historyCtx, historyCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer historyCancel()

		fromTime := time.Now().Add(-5 * time.Minute)
		toTime := time.Now()

		historyResp, err := m.client.GetTelemetryHistory(historyCtx, &hydrapb.TelemetryHistoryRequest{
			FromTime:           timestamppb.New(fromTime),
			ToTime:             timestamppb.New(toTime),
			ErrorsOnly:         false,
			FilterSwampPattern: "",
			Limit:              500,
		})
		if err == nil && historyResp != nil {
			return historyBatchMsg{events: historyResp.Events}
		}

		return nil
	}
}

// startStreaming starts the live event streaming
func (m Model) startStreaming() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}

		ctx := context.Background()
		stream, err := m.client.SubscribeToTelemetry(ctx, &hydrapb.TelemetrySubscribeRequest{
			ErrorsOnly:         false,
			IncludeSuccesses:   true,
			FilterSwampPattern: "",
		})
		if err != nil {
			return streamErrorMsg{err: err}
		}

		event, err := stream.Recv()
		if err != nil {
			return streamErrorMsg{err: err}
		}

		return streamEventMsg{event: event}
	}
}

// fetchStats fetches the current stats
func (m Model) fetchStats() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stats, err := m.client.GetTelemetryStats(ctx, &hydrapb.TelemetryStatsRequest{
			WindowMinutes: 5,
		})
		if err != nil {
			return nil
		}
		return statsMsg(stats)
	}
}

// View renders the TUI
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	if m.showErrorDetail && m.selectedError != nil {
		return m.renderErrorDetail()
	}

	return m.renderMain()
}

// renderMain renders the main view with fixed header and footer
func (m Model) renderMain() string {
	headerHeight := 4
	footerHeight := 2
	contentHeight := m.height - headerHeight - footerHeight
	if contentHeight < 5 {
		contentHeight = 5
	}

	// Header
	var header string
	title := titleStyle.Render(" HydrAIDE Observe ")
	status := ""
	if !m.connected {
		if m.connError != "" {
			status = errorStyle.Render(" ✗ " + m.connError)
		} else {
			status = " Connecting..."
		}
	} else if m.paused {
		status = pausedStyle.Render(" ⏸ PAUSED ")
	} else {
		status = successStyle.Render(" ● LIVE ")
	}
	header += title + "  " + status + "\n\n"
	header += m.renderTabs() + "\n"
	header += lipgloss.NewStyle().Foreground(mutedColor).Render(strings.Repeat("─", min(m.width-2, 100))) + "\n"

	// Content
	var content string
	switch m.activeTab {
	case TabLive:
		content = m.renderLiveTab(contentHeight)
	case TabErrors:
		content = m.renderErrorsTab(contentHeight)
	case TabStats:
		content = m.renderStatsTab()
	case TabLongRunning:
		content = m.renderLongRunningTab()
	}

	contentLines := strings.Count(content, "\n")
	for contentLines < contentHeight {
		content += "\n"
		contentLines++
	}

	// Footer
	footer := lipgloss.NewStyle().Foreground(mutedColor).Render(strings.Repeat("─", min(m.width-2, 100))) + "\n"
	footer += m.renderStatusBar()

	return header + content + footer
}

// renderTabs renders the tab bar
func (m Model) renderTabs() string {
	tabs := []struct {
		name string
		tab  Tab
	}{
		{"[1] Live", TabLive},
		{"[2] Errors", TabErrors},
		{"[3] Stats", TabStats},
		{"[4] Long", TabLongRunning},
	}

	var result string
	for _, t := range tabs {
		if t.tab == m.activeTab {
			result += activeTabStyle.Render(t.name) + "  "
		} else {
			result += inactiveTabStyle.Render(t.name) + "  "
		}
	}

	return result
}

// renderLiveTab renders the live events tab
func (m Model) renderLiveTab(maxHeight int) string {
	var filteredEvents []Event
	for _, e := range m.events {
		if e.SwampName != "" || !e.Success {
			filteredEvents = append(filteredEvents, e)
		}
	}

	if len(filteredEvents) == 0 {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("  No events yet. Waiting for activity...")
	}

	// Calculate swamp width same as in renderEventRow
	fixedWidth := 58
	swampWidth := m.width - fixedWidth
	if swampWidth < 30 {
		swampWidth = 30
	}

	// Header
	header := fmt.Sprintf("  %-12s %-12s %-*s %7s %s",
		"TIME", "METHOD", swampWidth, "SWAMP", "DURATION", "STATUS")

	var rows string
	rows += lipgloss.NewStyle().Foreground(mutedColor).Render(header) + "\n"

	visibleCount := maxHeight - 2
	if visibleCount < 3 {
		visibleCount = 3
	}

	startIdx := 0
	if len(filteredEvents) > visibleCount {
		startIdx = len(filteredEvents) - visibleCount
	}

	for i := startIdx; i < len(filteredEvents); i++ {
		event := filteredEvents[i]
		row := m.renderEventRow(event, false)
		rows += row + "\n"
	}

	return rows
}

// renderEventRow renders a single event row - plain text, no wrapping
func (m Model) renderEventRow(event Event, selected bool) string {
	timeStr := event.Timestamp.Format("15:04:05.000")

	// Pad method to 12 chars
	method := event.Method
	if len(method) > 12 {
		method = method[:12]
	}
	method = fmt.Sprintf("%-12s", method)

	// Calculate swamp width based on terminal width
	// Layout: prefix(2) + time(12) + space(1) + method(12) + space(1) + swamp(X) + space(1) + duration(7) + space(1) + status(20)
	fixedWidth := 58
	swampWidth := m.width - fixedWidth
	if swampWidth < 30 {
		swampWidth = 30
	}

	swampName := event.SwampName
	if swampName == "" {
		swampName = "-"
	}
	// Strip island ID from the beginning (e.g., "193/queueService/..." -> "queueService/...")
	if idx := strings.Index(swampName, "/"); idx != -1 {
		// Check if the part before "/" is a number (island ID)
		prefix := swampName[:idx]
		if _, err := strconv.ParseUint(prefix, 10, 64); err == nil {
			swampName = swampName[idx+1:]
		}
	}
	if len(swampName) > swampWidth {
		swampName = "…" + swampName[len(swampName)-swampWidth+1:]
	}
	swampName = fmt.Sprintf("%-*s", swampWidth, swampName)

	// Duration - right aligned, 7 chars
	duration := formatDuration(event.DurationUs)
	duration = fmt.Sprintf("%7s", duration)

	// Status - FailedPrecondition is INFO (not a real error), others are errors
	var status string
	if event.Success {
		status = successStyle.Render("OK")
	} else if event.ErrorCode == "FailedPrecondition" {
		status = warningStyle.Render("⚠ INFO")
	} else {
		errCode := event.ErrorCode
		if len(errCode) > 18 {
			errCode = errCode[:18]
		}
		status = errorStyle.Render("✗ " + errCode)
	}

	// Build row with colored parts but no width constraints
	prefix := "  "
	if selected {
		prefix = "▶ "
	}

	// Color method based on type
	coloredMethod := getMethodStyle(event.Method).Render(method)

	row := fmt.Sprintf("%s%s %s %s %s %s",
		prefix,
		timestampStyle.Render(timeStr),
		coloredMethod,
		swampName,
		durationStyle.Render(duration),
		status)

	return row
}

// renderErrorsTab renders the errors tab (excludes FailedPrecondition - those are INFO, not errors)
func (m Model) renderErrorsTab(maxHeight int) string {
	var errorEvents []Event
	for _, e := range m.events {
		// Only real errors, not FailedPrecondition (which is just INFO)
		if !e.Success && e.ErrorCode != "FailedPrecondition" {
			errorEvents = append(errorEvents, e)
		}
	}

	if len(errorEvents) == 0 {
		return successStyle.Render("  ✓ No errors recorded")
	}

	// Calculate swamp width same as in renderEventRow
	fixedWidth := 58
	swampWidth := m.width - fixedWidth
	if swampWidth < 30 {
		swampWidth = 30
	}

	var rows string
	rows += errorStyle.Render(fmt.Sprintf("  %d errors in current session", len(errorEvents))) + "\n\n"

	header := fmt.Sprintf("  %-12s %-12s %-*s %7s %s",
		"TIME", "METHOD", swampWidth, "SWAMP", "DURATION", "ERROR")
	rows += lipgloss.NewStyle().Foreground(mutedColor).Render(header) + "\n"

	visibleCount := maxHeight - 4
	if visibleCount < 3 {
		visibleCount = 3
	}

	startIdx := 0
	if len(errorEvents) > visibleCount {
		startIdx = len(errorEvents) - visibleCount
	}

	for i := startIdx; i < len(errorEvents); i++ {
		event := errorEvents[i]
		row := m.renderEventRow(event, false)
		rows += row + "\n"
	}

	return rows
}

// renderStatsTab renders the stats tab
func (m Model) renderStatsTab() string {
	if m.stats == nil {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("  Loading stats...")
	}

	s := m.stats

	// Count precondition failures vs real errors from top errors
	var preconditionCount int64
	var realErrorCount int64
	var preconditionErrors []*hydrapb.TelemetryErrorSummary
	var realErrors []*hydrapb.TelemetryErrorSummary

	for _, e := range s.TopErrors {
		if e.ErrorCode == "FailedPrecondition" {
			preconditionCount += e.Count
			preconditionErrors = append(preconditionErrors, e)
		} else {
			realErrorCount += e.Count
			realErrors = append(realErrors, e)
		}
	}

	var result string

	result += "  " + statLabelStyle.Render("Total Calls: ") + statValueStyle.Render(fmt.Sprintf("%d", s.TotalCalls)) + "\n"
	result += "  " + statLabelStyle.Render("Real Errors: ") + errorStyle.Render(fmt.Sprintf("%d", realErrorCount)) + "\n"
	result += "  " + statLabelStyle.Render("Precondition (INFO): ") + warningStyle.Render(fmt.Sprintf("%d", preconditionCount)) + "\n"
	result += "  " + statLabelStyle.Render("Avg Duration: ") + statValueStyle.Render(formatDuration(int64(s.AvgDurationUs))) + "\n"
	result += "  " + statLabelStyle.Render("Active Clients: ") + statValueStyle.Render(fmt.Sprintf("%d", s.ActiveClients)) + "\n"

	if len(s.TopSwamps) > 0 {
		result += "\n  " + lipgloss.NewStyle().Bold(true).Render("Top Swamps:") + "\n"
		for i, swamp := range s.TopSwamps {
			result += fmt.Sprintf("    %d. %s (%d calls)\n", i+1, swamp.SwampName, swamp.CallCount)
		}
	}

	// Show real errors first
	if len(realErrors) > 0 {
		result += "\n  " + errorDetailHeaderStyle.Render("Top Errors:") + "\n"
		for i, e := range realErrors {
			swampInfo := ""
			if e.LastSwamp != "" {
				swampInfo = fmt.Sprintf(" → %s", e.LastSwamp)
			}
			result += fmt.Sprintf("    %d. [%dx] %s: %s%s\n", i+1, e.Count, e.ErrorCode, e.ErrorMessage, swampInfo)
		}
	}

	// Show precondition failures separately (INFO, not errors)
	if len(preconditionErrors) > 0 {
		result += "\n  " + warningStyle.Render("Precondition Failures (INFO - not real errors):") + "\n"
		for i, e := range preconditionErrors {
			swampInfo := ""
			if e.LastSwamp != "" {
				swampInfo = fmt.Sprintf(" → %s", e.LastSwamp)
			}
			result += fmt.Sprintf("    %d. [%dx] %s%s\n", i+1, e.Count, e.ErrorMessage, swampInfo)
		}
	}

	return result
}

// renderLongRunningTab renders the long running operations tab
func (m Model) renderLongRunningTab() string {
	threshold := int64(5000000) // 5 seconds in microseconds

	var longRunning []Event
	for _, e := range m.events {
		if e.DurationUs >= threshold {
			longRunning = append(longRunning, e)
		}
	}

	var result string
	result += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("  ⏱ Long Running Operations (≥5s)") + "\n\n"

	if len(longRunning) == 0 {
		result += successStyle.Render("  ✓ No long running operations detected")
		return result
	}

	// Calculate swamp width - same formula but with extra space for client IP
	fixedWidth := 72
	swampWidth := m.width - fixedWidth
	if swampWidth < 30 {
		swampWidth = 30
	}

	header := fmt.Sprintf("  %-12s %-12s %-*s %10s %s",
		"TIME", "METHOD", swampWidth, "SWAMP", "DURATION", "CLIENT")
	result += lipgloss.NewStyle().Foreground(mutedColor).Render(header) + "\n"

	startIdx := 0
	if len(longRunning) > 20 {
		startIdx = len(longRunning) - 20
	}

	for i := startIdx; i < len(longRunning); i++ {
		e := longRunning[i]
		timeStr := e.Timestamp.Format("15:04:05.000")

		method := e.Method
		if len(method) > 12 {
			method = method[:12]
		}
		method = fmt.Sprintf("%-12s", method)

		swamp := e.SwampName
		if len(swamp) > swampWidth {
			swamp = "…" + swamp[len(swamp)-swampWidth+1:]
		}
		swamp = fmt.Sprintf("%-*s", swampWidth, swamp)

		durationStr := formatDuration(e.DurationUs)

		clientIP := e.ClientIP
		if len(clientIP) > 15 {
			clientIP = clientIP[:15]
		}

		var durationStyled string
		if e.DurationUs >= 10000000 { // 10s in µs
			durationStyled = errorStyle.Render(fmt.Sprintf("%10s", durationStr))
		} else {
			durationStyled = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(fmt.Sprintf("%10s", durationStr))
		}

		result += fmt.Sprintf("  %s %s %s %s %s\n",
			timestampStyle.Render(timeStr),
			getMethodStyle(e.Method).Render(method),
			swamp,
			durationStyled,
			clientIP)
	}

	result += "\n" + lipgloss.NewStyle().Foreground(mutedColor).Italic(true).Render(
		fmt.Sprintf("  Showing %d operations with duration ≥5s", len(longRunning)))

	return result
}

// renderErrorDetail renders the error detail view
func (m Model) renderErrorDetail() string {
	e := m.selectedError
	if e == nil {
		return "No error selected"
	}

	var s string
	s += errorDetailHeaderStyle.Render("  🔍 Error Details") + "\n\n"
	s += "  " + errorDetailLabelStyle.Render("Time:") + " " + errorDetailValueStyle.Render(e.Timestamp.Format("2006-01-02 15:04:05.000")) + "\n"
	s += "  " + errorDetailLabelStyle.Render("Method:") + " " + errorDetailValueStyle.Render(e.Method) + "\n"
	s += "  " + errorDetailLabelStyle.Render("Swamp:") + " " + errorDetailValueStyle.Render(e.SwampName) + "\n"
	s += "  " + errorDetailLabelStyle.Render("Keys:") + " " + errorDetailValueStyle.Render(fmt.Sprintf("%v", e.Keys)) + "\n"
	s += "  " + errorDetailLabelStyle.Render("Duration:") + " " + errorDetailValueStyle.Render(formatDuration(e.DurationUs)) + "\n"
	s += "  " + errorDetailLabelStyle.Render("Client IP:") + " " + errorDetailValueStyle.Render(e.ClientIP) + "\n"
	s += "\n  " + errorStyle.Render("Error Code: "+e.ErrorCode) + "\n"
	s += "  " + errorStyle.Render("Message: "+e.ErrorMessage) + "\n"
	s += "\n  " + helpStyle.Render("Press [ESC] to go back")

	return s
}

// renderStatusBar renders the bottom status bar
func (m Model) renderStatusBar() string {
	eventCount := fmt.Sprintf("%d events", len(m.events))

	var realErrorCount int
	var preconditionCount int
	for _, e := range m.events {
		if !e.Success {
			if e.ErrorCode == "FailedPrecondition" {
				preconditionCount++
			} else {
				realErrorCount++
			}
		}
	}

	var statusInfo string
	if realErrorCount > 0 {
		statusInfo = errorStyle.Render(fmt.Sprintf("%d errors", realErrorCount))
	} else {
		statusInfo = successStyle.Render("0 errors")
	}
	if preconditionCount > 0 {
		statusInfo += " | " + warningStyle.Render(fmt.Sprintf("%d info", preconditionCount))
	}

	pauseHint := "[P] Pause"
	if m.paused {
		pauseHint = "[P] Resume"
	}

	filter := ""
	if m.errorsOnly {
		filter = " [Errors Only]"
	}

	left := statusBarStyle.Render(eventCount + " | " + statusInfo + filter)
	right := helpStyle.Render(pauseHint + "  [?] Help  [Q] Quit")

	return left + "    " + right
}

// renderHelp renders the help screen
func (m Model) renderHelp() string {
	s := titleStyle.Render(" HydrAIDE Observe - Help ") + "\n\n"

	s += lipgloss.NewStyle().Bold(true).Render("  Navigation:") + "\n"
	s += "  " + keyStyle.Render("Up/k, Down/j") + keyDescStyle.Render("  Move selection up/down") + "\n"
	s += "  " + keyStyle.Render("Enter") + keyDescStyle.Render("         View error details") + "\n"
	s += "  " + keyStyle.Render("Esc") + keyDescStyle.Render("           Close detail view") + "\n"

	s += "\n" + lipgloss.NewStyle().Bold(true).Render("  Tabs:") + "\n"
	s += "  " + keyStyle.Render("1") + keyDescStyle.Render("  Live view") + "\n"
	s += "  " + keyStyle.Render("2") + keyDescStyle.Render("  Errors only") + "\n"
	s += "  " + keyStyle.Render("3") + keyDescStyle.Render("  Statistics") + "\n"
	s += "  " + keyStyle.Render("4") + keyDescStyle.Render("  Long running operations") + "\n"

	s += "\n" + lipgloss.NewStyle().Bold(true).Render("  Actions:") + "\n"
	s += "  " + keyStyle.Render("P") + keyDescStyle.Render("  Pause/Resume live stream") + "\n"
	s += "  " + keyStyle.Render("C") + keyDescStyle.Render("  Clear events") + "\n"
	s += "  " + keyStyle.Render("E") + keyDescStyle.Render("  Toggle errors only filter") + "\n"
	s += "  " + keyStyle.Render("?/H") + keyDescStyle.Render("  Toggle this help") + "\n"
	s += "  " + keyStyle.Render("Q") + keyDescStyle.Render("  Quit") + "\n"

	s += "\n  " + helpStyle.Render("Press any key to close help")

	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatDuration formats duration in microseconds to a human-readable string
// Shows µs for <1ms, ms for <1s, s for >=1s
func formatDuration(durationUs int64) string {
	if durationUs < 1000 {
		// Less than 1ms, show microseconds
		return fmt.Sprintf("%5dµs", durationUs)
	} else if durationUs < 1000000 {
		// Less than 1s, show milliseconds with one decimal
		ms := float64(durationUs) / 1000.0
		if ms < 10 {
			return fmt.Sprintf("%5.1fms", ms)
		}
		return fmt.Sprintf("%5.0fms", ms)
	} else {
		// 1s or more, show seconds with two decimals
		s := float64(durationUs) / 1000000.0
		return fmt.Sprintf("%5.2fs", s)
	}
}
