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

	"github.com/cespare/xxhash/v2"
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
	selectedIdx int // Index in the FILTERED list, not the original events
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

	// Filters
	filterMethod string // Filter by method name (exact match)
	filterSwamp  string // Filter by swamp name (partial match)
	filterStatus string // Filter by status: "all", "ok", "error", "info"
	showFilter   bool   // Show filter input mode

	// Inspect mode
	showInspect        bool
	inspectEvent       *Event
	inspectTreasures   []*hydrapb.Treasure
	inspectPage        int
	inspectPerPage     int
	inspectTotal       int
	inspectLoading     bool
	inspectError       string
	inspectSelectedIdx int // Selected row in inspect view
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

type inspectDataMsg struct {
	treasures []*hydrapb.Treasure
	total     int
}

type inspectErrorMsg struct {
	err error
}

// NewModel creates a new observe model
func NewModel(serverAddr, certFile, keyFile, caFile string) Model {
	return Model{
		maxEvents:      500,
		events:         make([]Event, 0, 500),
		pauseBuffer:    make([]Event, 0),
		activeTab:      TabLive,
		serverAddr:     serverAddr,
		certFile:       certFile,
		keyFile:        keyFile,
		caFile:         caFile,
		inspectPerPage: 20,
		filterStatus:   "all",
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

	case inspectDataMsg:
		m.inspectLoading = false
		m.inspectTreasures = msg.treasures
		m.inspectTotal = msg.total

	case inspectErrorMsg:
		m.inspectLoading = false
		m.inspectError = msg.err.Error()
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
		if m.showInspect {
			// In inspect mode: navigate rows or pages
			if m.inspectSelectedIdx > 0 {
				m.inspectSelectedIdx--
			} else if m.inspectPage > 0 {
				m.inspectPage--
				m.inspectSelectedIdx = m.inspectPerPage - 1
				return m, m.fetchInspectData()
			}
		} else {
			// Navigate in filtered events list
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}
		}
	case "down", "j":
		if m.showInspect {
			// In inspect mode: navigate rows or pages
			if m.inspectSelectedIdx < len(m.inspectTreasures)-1 {
				m.inspectSelectedIdx++
			} else if (m.inspectPage+1)*m.inspectPerPage < m.inspectTotal {
				m.inspectPage++
				m.inspectSelectedIdx = 0
				return m, m.fetchInspectData()
			}
		} else {
			// Navigate in filtered events list
			filtered := m.getFilteredEvents()
			if m.selectedIdx < len(filtered)-1 {
				m.selectedIdx++
			}
		}
	case "pgup":
		if m.showInspect {
			if m.inspectPage > 0 {
				m.inspectPage--
				m.inspectSelectedIdx = 0
				return m, m.fetchInspectData()
			}
		} else {
			// Page up in events - jump by visible count
			pageSize := m.height - 10
			if pageSize < 5 {
				pageSize = 5
			}
			m.selectedIdx -= pageSize
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
		}
	case "pgdown":
		if m.showInspect {
			if (m.inspectPage+1)*m.inspectPerPage < m.inspectTotal {
				m.inspectPage++
				m.inspectSelectedIdx = 0
				return m, m.fetchInspectData()
			}
		} else {
			// Page down in events
			filtered := m.getFilteredEvents()
			pageSize := m.height - 10
			if pageSize < 5 {
				pageSize = 5
			}
			m.selectedIdx += pageSize
			if m.selectedIdx >= len(filtered) {
				m.selectedIdx = len(filtered) - 1
			}
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
		}
	case "home":
		m.selectedIdx = 0
		m.inspectSelectedIdx = 0
		if m.showInspect && m.inspectPage > 0 {
			m.inspectPage = 0
			return m, m.fetchInspectData()
		}
	case "end":
		if m.showInspect {
			// Go to last page
			lastPage := (m.inspectTotal - 1) / m.inspectPerPage
			if m.inspectPage != lastPage {
				m.inspectPage = lastPage
				return m, m.fetchInspectData()
			}
			m.inspectSelectedIdx = len(m.inspectTreasures) - 1
		} else {
			filtered := m.getFilteredEvents()
			m.selectedIdx = len(filtered) - 1
			if m.selectedIdx < 0 {
				m.selectedIdx = 0
			}
		}
	case "enter":
		if m.showInspect {
			// In inspect mode: show full value of selected treasure
			if m.inspectSelectedIdx >= 0 && m.inspectSelectedIdx < len(m.inspectTreasures) {
				// TODO: Show full value detail view
			}
			return m, nil
		}
		// Open inspect for selected event (if it has a swamp name)
		filtered := m.getFilteredEvents()
		if m.selectedIdx >= 0 && m.selectedIdx < len(filtered) {
			event := filtered[m.selectedIdx]
			if event.SwampName != "" && event.SwampName != "-" {
				m.inspectEvent = &event
				m.showInspect = true
				m.inspectLoading = true
				m.inspectPage = 0
				m.inspectSelectedIdx = 0
				m.inspectTreasures = nil
				m.inspectError = ""
				return m, m.fetchInspectData()
			}
		}
	case "esc":
		if m.showInspect {
			// Close inspect mode
			m.showInspect = false
			m.inspectEvent = nil
			m.inspectTreasures = nil
			m.inspectError = ""
			m.inspectSelectedIdx = 0
			return m, nil
		}
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
	}

	// Auto-scroll to bottom when not paused
	if !m.paused {
		filtered := m.getFilteredEvents()
		m.selectedIdx = len(filtered) - 1
		if m.selectedIdx < 0 {
			m.selectedIdx = 0
		}
	}
}

// getFilteredEvents returns events filtered by current filters
func (m *Model) getFilteredEvents() []Event {
	var filtered []Event
	for _, e := range m.events {
		// Skip events without swamp name in Live tab (unless it's an error)
		if m.activeTab == TabLive && e.SwampName == "" && e.Success {
			continue
		}

		// Apply method filter
		if m.filterMethod != "" && e.Method != m.filterMethod {
			continue
		}

		// Apply swamp filter (partial match, case-insensitive)
		if m.filterSwamp != "" && !strings.Contains(strings.ToLower(e.SwampName), strings.ToLower(m.filterSwamp)) {
			continue
		}

		// Apply status filter
		switch m.filterStatus {
		case "ok":
			if !e.Success {
				continue
			}
		case "error":
			if e.Success || e.ErrorCode == "FailedPrecondition" {
				continue
			}
		case "info":
			if e.ErrorCode != "FailedPrecondition" {
				continue
			}
		}

		// For errors tab, only show real errors
		if m.activeTab == TabErrors {
			if e.Success || e.ErrorCode == "FailedPrecondition" {
				continue
			}
		}

		filtered = append(filtered, e)
	}
	return filtered
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

	if m.showInspect {
		return m.renderInspect()
	}

	if m.showErrorDetail && m.selectedError != nil {
		return m.renderErrorDetail()
	}

	return m.renderMain()
}

// fetchInspectData fetches treasure data for the selected swamp using GetByIndex
func (m Model) fetchInspectData() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil || m.inspectEvent == nil {
			return inspectErrorMsg{err: fmt.Errorf("no client or event")}
		}

		// Parse island ID and swamp name from SwampName (format: "193/sanctuary/realm/swamp")
		swampFullName := m.inspectEvent.SwampName
		var islandID uint64
		var swampName string

		if idx := strings.Index(swampFullName, "/"); idx != -1 {
			prefix := swampFullName[:idx]
			if id, err := strconv.ParseUint(prefix, 10, 64); err == nil {
				islandID = id
				swampName = swampFullName[idx+1:]
			} else {
				swampName = swampFullName
			}
		} else {
			swampName = swampFullName
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := m.client.GetByIndex(ctx, &hydrapb.GetByIndexRequest{
			IslandID:  islandID,
			SwampName: swampName,
			IndexType: hydrapb.IndexType_KEY,
			OrderType: hydrapb.OrderType_ASC,
			From:      int32(m.inspectPage * m.inspectPerPage),
			Limit:     int32(m.inspectPerPage),
		})
		if err != nil {
			return inspectErrorMsg{err: err}
		}

		// Get total count (we'll estimate from the response)
		total := len(resp.Treasures)
		if total == m.inspectPerPage {
			// There might be more, estimate higher
			total = (m.inspectPage + 2) * m.inspectPerPage
		} else {
			total = m.inspectPage*m.inspectPerPage + len(resp.Treasures)
		}

		return inspectDataMsg{
			treasures: resp.Treasures,
			total:     total,
		}
	}
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
	filteredEvents := m.getFilteredEvents()

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

	// selectedIdx is now the index in the filtered list directly
	selectedIdx := m.selectedIdx
	if selectedIdx >= len(filteredEvents) {
		selectedIdx = len(filteredEvents) - 1
	}
	if selectedIdx < 0 {
		selectedIdx = 0
	}

	// Calculate visible window around selection
	startIdx := 0
	endIdx := len(filteredEvents)

	if len(filteredEvents) > visibleCount {
		// Center the selection in the visible area
		startIdx = selectedIdx - visibleCount/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + visibleCount
		if endIdx > len(filteredEvents) {
			endIdx = len(filteredEvents)
			startIdx = endIdx - visibleCount
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	for i := startIdx; i < endIdx; i++ {
		event := filteredEvents[i]
		isSelected := i == selectedIdx
		row := m.renderEventRow(event, isSelected)
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

	// Highlight selected row with background
	if selected {
		row = selectedRowStyle.Render(row)
	}

	return row
}

// renderErrorsTab renders the errors tab (excludes FailedPrecondition - those are INFO, not errors)
func (m Model) renderErrorsTab(maxHeight int) string {
	// getFilteredEvents already filters for errors when activeTab == TabErrors
	errorEvents := m.getFilteredEvents()

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

	// selectedIdx is now the index in the filtered list
	selectedIdx := m.selectedIdx
	if selectedIdx >= len(errorEvents) {
		selectedIdx = len(errorEvents) - 1
	}
	if selectedIdx < 0 {
		selectedIdx = 0
	}

	// Calculate visible window
	startIdx := 0
	endIdx := len(errorEvents)

	if len(errorEvents) > visibleCount {
		startIdx = selectedIdx - visibleCount/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + visibleCount
		if endIdx > len(errorEvents) {
			endIdx = len(errorEvents)
			startIdx = endIdx - visibleCount
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	for i := startIdx; i < endIdx; i++ {
		event := errorEvents[i]
		isSelected := i == selectedIdx
		row := m.renderEventRow(event, isSelected)
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

// renderInspect renders the swamp inspection view
func (m Model) renderInspect() string {
	if m.inspectEvent == nil {
		return "No swamp selected"
	}

	// Parse island ID and swamp name from SwampName (format: "193/sanctuary/realm/swamp")
	swampFullName := m.inspectEvent.SwampName
	var islandID uint64
	swampDisplayName := swampFullName

	if idx := strings.Index(swampFullName, "/"); idx != -1 {
		prefix := swampFullName[:idx]
		if id, err := strconv.ParseUint(prefix, 10, 64); err == nil {
			islandID = id
			swampDisplayName = swampFullName[idx+1:]
		}
	}

	// Calculate swamp path using name package logic
	// Path format: islandID/hash1/hash2/swampHash.hyd
	swampPath := calculateSwampPath(swampDisplayName, islandID)

	var s string
	s += titleStyle.Render(" 🔍 Swamp Inspector ") + "\n\n"
	s += "  " + statLabelStyle.Render("Swamp: ") + statValueStyle.Render(swampDisplayName) + "\n"
	s += "  " + statLabelStyle.Render("Path:  ") + lipgloss.NewStyle().Foreground(mutedColor).Render(swampPath) + "\n"
	s += "  " + statLabelStyle.Render("Method: ") + getMethodStyle(m.inspectEvent.Method).Render(m.inspectEvent.Method) + "\n"
	s += "  " + statLabelStyle.Render("Time: ") + timestampStyle.Render(m.inspectEvent.Timestamp.Format("15:04:05.000")) + "\n\n"

	if m.inspectLoading {
		s += lipgloss.NewStyle().Foreground(mutedColor).Render("  Loading treasures...") + "\n"
		s += "\n  " + helpStyle.Render("Press [ESC] to go back")
		return s
	}

	if m.inspectError != "" {
		s += errorStyle.Render("  ✗ Error: "+m.inspectError) + "\n"
		s += "\n  " + helpStyle.Render("Press [ESC] to go back")
		return s
	}

	if len(m.inspectTreasures) == 0 {
		s += warningStyle.Render("  ⚠ No treasures found in this swamp") + "\n"
		s += "\n  " + helpStyle.Render("Press [ESC] to go back")
		return s
	}

	// Header with page info
	s += lipgloss.NewStyle().Bold(true).Render("  Treasures:") + "\n"
	s += lipgloss.NewStyle().Foreground(mutedColor).Render(
		fmt.Sprintf("  Page %d | Showing %d items | Use ↑/↓ to navigate", m.inspectPage+1, len(m.inspectTreasures))) + "\n\n"

	// Dynamic column widths based on terminal width
	// Layout: prefix(2) + key(X) + space(1) + type(10) + space(1) + created(17) + space(1) + value(rest)
	fixedWidth := 35 // prefix + type + spaces + created
	keyWidth := (m.width - fixedWidth) / 3
	if keyWidth < 15 {
		keyWidth = 15
	}
	if keyWidth > 40 {
		keyWidth = 40
	}
	valueWidth := m.width - fixedWidth - keyWidth - 5
	if valueWidth < 20 {
		valueWidth = 20
	}

	// Table header
	header := fmt.Sprintf("  %-*s %-10s %-17s %s", keyWidth, "KEY", "TYPE", "CREATED", "VALUE")
	s += lipgloss.NewStyle().Foreground(mutedColor).Render(header) + "\n"
	s += lipgloss.NewStyle().Foreground(mutedColor).Render("  "+strings.Repeat("─", min(m.width-4, 100))) + "\n"

	// Treasure list with selection
	for i, t := range m.inspectTreasures {
		if !t.IsExist {
			continue
		}

		isSelected := i == m.inspectSelectedIdx

		key := t.Key
		if len(key) > keyWidth-2 {
			key = key[:keyWidth-5] + "..."
		}
		key = fmt.Sprintf("%-*s", keyWidth, key)

		valueType, valueStr := getTreasureTypeAndValue(t)
		if len(valueStr) > valueWidth {
			valueStr = valueStr[:valueWidth-3] + "..."
		}

		createdAt := "-"
		if t.CreatedAt != nil {
			createdAt = t.CreatedAt.AsTime().Format("2006-01-02 15:04")
		}

		prefix := "  "
		if isSelected {
			prefix = "▶ "
		}

		row := fmt.Sprintf("%s%s %-10s %-17s %s",
			prefix,
			statValueStyle.Render(key),
			lipgloss.NewStyle().Foreground(secondaryColor).Render(valueType),
			timestampStyle.Render(createdAt),
			valueStr)

		if isSelected {
			row = selectedRowStyle.Render(row)
		}
		s += row + "\n"
	}

	s += "\n  " + helpStyle.Render("[↑/↓] Navigate  [PgUp/PgDn] Page  [ESC] Back")

	return s
}

// calculateSwampPath generates the expected file path for a swamp
// Using the same hash algorithm as the name package
func calculateSwampPath(swampName string, islandID uint64) string {
	// Default depth and maxFoldersPerLevel (common settings)
	depth := 2
	maxFoldersPerLevel := 256

	// Generate hash for directory path
	hash := xxhash.Sum64String(swampName)
	hashHex := fmt.Sprintf("%x", hash)

	charsPerLevel := 2 // For 256 folders

	parts := make([]string, depth)
	for i := 0; i < depth; i++ {
		start := i * charsPerLevel
		end := start + charsPerLevel
		if end > len(hashHex) {
			end = len(hashHex)
		}
		if start < len(hashHex) {
			parts[i] = hashHex[start:end]
		}
	}

	// Generate swamp folder name (hash of full name)
	swampHash := fmt.Sprintf("%x", xxhash.Sum64([]byte(swampName)))

	// Combine: islandID/hash1/hash2/swampHash.hyd
	return fmt.Sprintf("%d/%s/%s.hyd", islandID, strings.Join(parts, "/"), swampHash)
}

// getTreasureTypeAndValue returns the type and value of a treasure for display
func getTreasureTypeAndValue(t *hydrapb.Treasure) (string, string) {
	// Check each possible value type
	if t.Int8Val != nil {
		return "int8", fmt.Sprintf("%d", *t.Int8Val)
	}
	if t.Int16Val != nil {
		return "int16", fmt.Sprintf("%d", *t.Int16Val)
	}
	if t.Int32Val != nil {
		return "int32", fmt.Sprintf("%d", *t.Int32Val)
	}
	if t.Int64Val != nil {
		return "int64", fmt.Sprintf("%d", *t.Int64Val)
	}
	if t.Uint8Val != nil {
		return "uint8", fmt.Sprintf("%d", *t.Uint8Val)
	}
	if t.Uint16Val != nil {
		return "uint16", fmt.Sprintf("%d", *t.Uint16Val)
	}
	if t.Uint32Val != nil {
		return "uint32", fmt.Sprintf("%d", *t.Uint32Val)
	}
	if t.Uint64Val != nil {
		return "uint64", fmt.Sprintf("%d", *t.Uint64Val)
	}
	if t.Float32Val != nil {
		return "float32", fmt.Sprintf("%.4f", *t.Float32Val)
	}
	if t.Float64Val != nil {
		return "float64", fmt.Sprintf("%.4f", *t.Float64Val)
	}
	if t.StringVal != nil {
		val := *t.StringVal
		if len(val) > 40 {
			val = val[:37] + "..."
		}
		return "string", fmt.Sprintf("\"%s\"", val)
	}
	if t.BoolVal != nil {
		if *t.BoolVal == hydrapb.Boolean_TRUE {
			return "bool", "true"
		}
		return "bool", "false"
	}
	if t.BytesVal != nil {
		return "bytes", fmt.Sprintf("(%d bytes)", len(t.BytesVal))
	}
	if len(t.Uint32Slice) > 0 {
		return "uint32[]", fmt.Sprintf("[%d items]", len(t.Uint32Slice))
	}

	return "unknown", "-"
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
	s += "  " + keyStyle.Render("↑/k, ↓/j") + keyDescStyle.Render("      Move selection up/down") + "\n"
	s += "  " + keyStyle.Render("PgUp/PgDn") + keyDescStyle.Render("     Jump one page up/down") + "\n"
	s += "  " + keyStyle.Render("Home/End") + keyDescStyle.Render("      Jump to first/last item") + "\n"
	s += "  " + keyStyle.Render("Enter") + keyDescStyle.Render("         Inspect swamp contents") + "\n"
	s += "  " + keyStyle.Render("Esc") + keyDescStyle.Render("           Close inspect/detail view") + "\n"

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

	s += "\n" + lipgloss.NewStyle().Bold(true).Render("  Inspect Mode:") + "\n"
	s += "  " + keyDescStyle.Render("  Select an event and press Enter to view swamp contents.") + "\n"
	s += "  " + keyDescStyle.Render("  Use ↑/↓ to navigate rows, PgUp/PgDn for pages.") + "\n"
	s += "  " + keyDescStyle.Render("  The swamp path is shown for debugging.") + "\n"

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
