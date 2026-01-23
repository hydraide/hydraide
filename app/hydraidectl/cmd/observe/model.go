package observe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
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
)

// Model is the Bubbletea model for the observe TUI
type Model struct {
	// Connection
	conn       *grpc.ClientConn
	client     hydrapb.HydraideServiceClient
	cancelFunc context.CancelFunc
	connected  bool
	connError  string

	// Events
	events      []Event
	selectedIdx int
	maxEvents   int
	paused      bool
	pauseBuffer []Event

	// Tabs
	activeTab Tab

	// Viewport for scrolling
	eventsViewport viewport.Model

	// Stats
	stats *hydrapb.TelemetryStatsResponse

	// Error details
	selectedError   *Event
	showErrorDetail bool

	// Filters
	errorsOnly  bool
	swampFilter string

	// Screen dimensions
	width  int
	height int

	// Help
	showHelp bool

	// Config
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
	DurationMs   int64
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

// historyBatchMsg contains a batch of historical events
type historyBatchMsg struct {
	events []*hydrapb.TelemetryEvent
}

// streamEventMsg is a single event from the live stream
type streamEventMsg struct {
	event *hydrapb.TelemetryEvent
}

// streamErrorMsg indicates a stream error
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
				DurationMs:   protoEvent.DurationMs,
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
			DurationMs:   protoEvent.DurationMs,
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

// subscribeToTelemetry starts the telemetry subscription and streams events
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

// renderMain renders the main view
func (m Model) renderMain() string {
	var s string

	title := titleStyle.Render(" HydrAIDE Observe ")
	status := ""
	if !m.connected {
		if m.connError != "" {
			status = errorStyle.Render(" X " + m.connError)
		} else {
			status = " Connecting..."
		}
	} else if m.paused {
		status = pausedStyle.Render(" PAUSED ")
	} else {
		status = successStyle.Render(" LIVE ")
	}

	s += title + "  " + status + "\n\n"
	s += m.renderTabs() + "\n\n"

	switch m.activeTab {
	case TabLive:
		s += m.renderLiveTab()
	case TabErrors:
		s += m.renderErrorsTab()
	case TabStats:
		s += m.renderStatsTab()
	}

	s += "\n" + m.renderStatusBar()

	return s
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
	}

	var result string
	for _, t := range tabs {
		if t.tab == m.activeTab {
			result += activeTabStyle.Render(t.name) + " "
		} else {
			result += inactiveTabStyle.Render(t.name) + " "
		}
	}

	return result
}

// renderLiveTab renders the live events tab
func (m Model) renderLiveTab() string {
	if len(m.events) == 0 {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("No events yet. Waiting for activity...\n")
	}

	var rows string

	header := fmt.Sprintf("%-12s %-8s %-40s %8s %s",
		"TIME", "METHOD", "SWAMP", "DURATION", "STATUS")
	rows += lipgloss.NewStyle().Foreground(mutedColor).Render(header) + "\n"
	rows += lipgloss.NewStyle().Foreground(mutedColor).Render(
		"-------------------------------------------------------------------") + "\n"

	visibleCount := m.height - 15
	if visibleCount < 5 {
		visibleCount = 5
	}

	startIdx := 0
	if m.selectedIdx >= visibleCount {
		startIdx = m.selectedIdx - visibleCount + 1
	}

	endIdx := startIdx + visibleCount
	if endIdx > len(m.events) {
		endIdx = len(m.events)
	}

	for i := startIdx; i < endIdx; i++ {
		event := m.events[i]
		row := m.renderEventRow(event, i == m.selectedIdx)
		rows += row + "\n"
	}

	return rows
}

// renderEventRow renders a single event row
func (m Model) renderEventRow(event Event, selected bool) string {
	timeStr := event.Timestamp.Format("15:04:05.000")
	methodStyle := getMethodStyle(event.Method)
	method := methodStyle.Render(event.Method)

	swampName := event.SwampName
	if len(swampName) > 40 {
		swampName = swampName[:37] + "..."
	}

	duration := fmt.Sprintf("%dms", event.DurationMs)

	var status string
	if event.Success {
		status = successStyle.Render("OK")
	} else {
		status = errorStyle.Render("X " + event.ErrorCode)
	}

	row := fmt.Sprintf("%s %s %-40s %8s %s",
		timestampStyle.Render(timeStr),
		method,
		swampStyle.Render(swampName),
		durationStyle.Render(duration),
		status)

	if selected {
		return selectedRowStyle.Render("> " + row)
	}
	return eventRowStyle.Render("  " + row)
}

// renderErrorsTab renders the errors tab
func (m Model) renderErrorsTab() string {
	var errorCount int
	for _, e := range m.events {
		if !e.Success {
			errorCount++
		}
	}

	if errorCount == 0 {
		return successStyle.Render("No errors recorded\n")
	}

	var rows string
	rows += errorStyle.Render(fmt.Sprintf("%d errors in current session\n\n", errorCount))

	for i := len(m.events) - 1; i >= 0 && i > len(m.events)-50; i-- {
		event := m.events[i]
		if !event.Success {
			row := m.renderEventRow(event, i == m.selectedIdx)
			rows += row + "\n"
		}
	}

	return rows
}

// renderStatsTab renders the stats tab
func (m Model) renderStatsTab() string {
	if m.stats == nil {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("Loading stats...\n")
	}

	s := m.stats
	var result string

	result += statLabelStyle.Render("Total Calls: ") + statValueStyle.Render(fmt.Sprintf("%d", s.TotalCalls)) + "\n"
	result += statLabelStyle.Render("Errors: ") + statValueStyle.Render(fmt.Sprintf("%d", s.ErrorCount)) + "\n"
	result += statLabelStyle.Render("Error Rate: ") + statValueStyle.Render(fmt.Sprintf("%.2f%%", s.ErrorRate)) + "\n"
	result += statLabelStyle.Render("Avg Duration: ") + statValueStyle.Render(fmt.Sprintf("%.2fms", s.AvgDurationMs)) + "\n"
	result += statLabelStyle.Render("Active Clients: ") + statValueStyle.Render(fmt.Sprintf("%d", s.ActiveClients)) + "\n"

	if len(s.TopSwamps) > 0 {
		result += "\n" + lipgloss.NewStyle().Bold(true).Render("Top Swamps:") + "\n"
		for i, swamp := range s.TopSwamps {
			result += fmt.Sprintf("  %d. %s (%d calls)\n", i+1, swamp.SwampName, swamp.CallCount)
		}
	}

	if len(s.TopErrors) > 0 {
		result += "\n" + errorDetailHeaderStyle.Render("Top Errors:") + "\n"
		for i, e := range s.TopErrors {
			swampInfo := ""
			if e.LastSwamp != "" {
				swampInfo = fmt.Sprintf(" -> %s", e.LastSwamp)
			}
			result += fmt.Sprintf("  %d. [%dx] %s: %s%s\n", i+1, e.Count, e.ErrorCode, e.ErrorMessage, swampInfo)
		}
	}

	return result
}

// renderErrorDetail renders the error detail view
func (m Model) renderErrorDetail() string {
	e := m.selectedError
	if e == nil {
		return "No error selected"
	}

	var s string
	s += errorDetailHeaderStyle.Render("Error Details") + "\n\n"
	s += errorDetailLabelStyle.Render("Time:") + " " + errorDetailValueStyle.Render(e.Timestamp.Format("2006-01-02 15:04:05.000")) + "\n"
	s += errorDetailLabelStyle.Render("Method:") + " " + errorDetailValueStyle.Render(e.Method) + "\n"
	s += errorDetailLabelStyle.Render("Swamp:") + " " + errorDetailValueStyle.Render(e.SwampName) + "\n"
	s += errorDetailLabelStyle.Render("Keys:") + " " + errorDetailValueStyle.Render(fmt.Sprintf("%v", e.Keys)) + "\n"
	s += errorDetailLabelStyle.Render("Duration:") + " " + errorDetailValueStyle.Render(fmt.Sprintf("%dms", e.DurationMs)) + "\n"
	s += errorDetailLabelStyle.Render("Client IP:") + " " + errorDetailValueStyle.Render(e.ClientIP) + "\n"
	s += "\n" + errorStyle.Render("Error Code: "+e.ErrorCode) + "\n"
	s += errorStyle.Render("Message: "+e.ErrorMessage) + "\n"
	s += "\n" + helpStyle.Render("Press [ESC] to go back")

	return s
}

// renderStatusBar renders the bottom status bar
func (m Model) renderStatusBar() string {
	eventCount := fmt.Sprintf("%d events", len(m.events))

	var errorCount int
	for _, e := range m.events {
		if !e.Success {
			errorCount++
		}
	}
	errors := fmt.Sprintf("%d errors", errorCount)

	filter := ""
	if m.errorsOnly {
		filter = " [Errors Only]"
	}

	help := "Press [?] for help  [Q] to quit"

	left := statusBarStyle.Render(eventCount + " | " + errors + filter)
	right := helpStyle.Render(help)

	return left + "  " + right
}

// renderHelp renders the help screen
func (m Model) renderHelp() string {
	s := titleStyle.Render(" HydrAIDE Observe - Help ") + "\n\n"

	s += lipgloss.NewStyle().Bold(true).Render("Navigation:") + "\n"
	s += keyStyle.Render("  Up/k, Down/j") + keyDescStyle.Render("  Move selection up/down") + "\n"
	s += keyStyle.Render("  Enter") + keyDescStyle.Render("    View error details") + "\n"
	s += keyStyle.Render("  Esc") + keyDescStyle.Render("      Close detail view") + "\n"

	s += "\n" + lipgloss.NewStyle().Bold(true).Render("Tabs:") + "\n"
	s += keyStyle.Render("  1") + keyDescStyle.Render("  Live view") + "\n"
	s += keyStyle.Render("  2") + keyDescStyle.Render("  Errors only") + "\n"
	s += keyStyle.Render("  3") + keyDescStyle.Render("  Statistics") + "\n"

	s += "\n" + lipgloss.NewStyle().Bold(true).Render("Actions:") + "\n"
	s += keyStyle.Render("  P") + keyDescStyle.Render("  Pause/Resume stream") + "\n"
	s += keyStyle.Render("  C") + keyDescStyle.Render("  Clear events") + "\n"
	s += keyStyle.Render("  E") + keyDescStyle.Render("  Toggle errors only filter") + "\n"
	s += keyStyle.Render("  ?/H") + keyDescStyle.Render("  Toggle this help") + "\n"
	s += keyStyle.Render("  Q") + keyDescStyle.Render("  Quit") + "\n"

	s += "\n" + helpStyle.Render("Press any key to close help")

	return s
}
