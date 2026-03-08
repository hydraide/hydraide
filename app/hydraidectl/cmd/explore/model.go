package explore

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hydraide/hydraide/app/server/explorer"
)

// viewLevel represents the current drill-down level.
type viewLevel int

const (
	levelSanctuaries viewLevel = iota
	levelRealms
	levelSwamps
	levelDetail
)

// scanDoneMsg is sent when the initial scan completes.
type scanDoneMsg struct {
	duration time.Duration
	files    int64
	errors   int64
	err      error
}

// scanTickMsg is sent periodically during scanning to update progress.
type scanTickMsg struct {
	scanned int64
	total   int64
	errors  int64
}

// deleteLevel describes what scope of deletion is being performed.
type deleteLevel int

const (
	deleteSanctuary deleteLevel = iota
	deleteRealm
	deleteSwamp
)

// deleteProgressMsg is sent periodically during deletion to update progress.
type deleteProgressMsg struct {
	destroyed int64
	failed    int64
	total     int64
	lastError string
}

// deleteDoneMsg is sent when the bulk deletion finishes.
type deleteDoneMsg struct {
	destroyed int64
	failed    int64
	duration  time.Duration
	lastError string
}

// Model is the Bubbletea model for the explore TUI.
type Model struct {
	explorer *explorer.Explorer
	dataPath string

	// Instance connection info (empty = offline/read-only mode)
	instanceName string
	basePath     string

	level     viewLevel
	cursor    int
	scrollOff int // scroll offset for long lists

	// Current data at each level
	sanctuaries []*explorer.SanctuaryInfo
	realms      []*explorer.RealmInfo
	swamps      []*explorer.SwampDetail
	detail      *explorer.SwampDetail

	// Navigation state
	selectedSanctuary string
	selectedRealm     string

	// Scan state
	scanning  bool
	scanInfo  string
	scanError string

	// Terminal size
	width  int
	height int

	// Search/filter
	searching  bool
	searchText string

	// Delete state
	deleteMode       viewLevel     // which level initiated the delete (levelSanctuaries/levelRealms/levelSwamps/levelDetail)
	deleteTargetName string        // human-readable target name
	deleteSwampList  []*explorer.SwampDetail // all swamps to delete
	deleteConfirm    int           // 0=not deleting, 1=first confirm, 2=second confirm
	deleteCode       string        // generated confirmation code
	deleteInput      string        // user-typed code
	deleting         bool          // deletion in progress
	deleteProgress   deleteProgressMsg
	deleteTotal      int64
	deleteDone       bool
	deleteResult     deleteDoneMsg
	deleteError      string        // error message if delete unavailable
	deleteUpdateCh   chan tea.Msg  // channel for progress/done messages from gRPC goroutine
}

// NewModel creates a new explore TUI model.
// If instanceName and basePath are non-empty, deletion is available via the running server.
func NewModel(dataPath, instanceName, basePath string) Model {
	return Model{
		explorer:     explorer.New(dataPath),
		dataPath:     dataPath,
		instanceName: instanceName,
		basePath:     basePath,
		level:        levelSanctuaries,
		scanning:     true,
		scanInfo:     "Scanning...",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.startScan(), m.scanTick())
}

func (m Model) startScan() tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		err := m.explorer.Scan(context.Background())
		status := m.explorer.GetScanStatus()
		return scanDoneMsg{
			duration: time.Since(start),
			files:    status.ScannedFiles,
			errors:   status.ErrorCount,
			err:      err,
		}
	}
}

func (m Model) scanTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
		status := m.explorer.GetScanStatus()
		return scanTickMsg{
			scanned: status.ScannedFiles,
			total:   status.TotalFiles,
			errors:  status.ErrorCount,
		}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case scanTickMsg:
		if m.scanning {
			if msg.total > 0 {
				m.scanInfo = fmt.Sprintf("Scanning... %d / %d files", msg.scanned, msg.total)
			} else {
				m.scanInfo = fmt.Sprintf("Scanning... %d files found", msg.scanned)
			}
			if msg.errors > 0 {
				m.scanInfo += fmt.Sprintf(" (%d errors)", msg.errors)
			}
			return m, m.scanTick()
		}
		return m, nil

	case scanDoneMsg:
		m.scanning = false
		if msg.err != nil {
			m.scanError = msg.err.Error()
		} else {
			m.scanInfo = fmt.Sprintf("%d files scanned in %s (%d errors)",
				msg.files, msg.duration.Round(time.Millisecond), msg.errors)
		}
		m.sanctuaries = m.explorer.ListSanctuaries()
		return m, nil

	case deleteProgressMsg:
		m.deleteProgress = msg
		return m, waitForDeleteUpdate(m.deleteUpdateCh)

	case deleteDoneMsg:
		m.deleting = false
		m.deleteDone = true
		m.deleteResult = msg
		return m, nil

	case tea.KeyMsg:
		if m.scanning {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		// Delete done — any key returns to normal view + rescan
		if m.deleteDone {
			m.deleteDone = false
			m.deleteConfirm = 0
			m.deleteError = ""
			m.scanning = true
			m.scanError = ""
			return m, tea.Batch(m.startScan(), m.scanTick())
		}

		// Deleting in progress — block input
		if m.deleting {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		// Delete confirmation input
		if m.deleteConfirm > 0 {
			return m.handleDeleteConfirmInput(msg)
		}

		// Delete error shown — any key dismisses
		if m.deleteError != "" {
			m.deleteError = ""
			return m, nil
		}

		// Search mode input handling
		if m.searching {
			return m.handleSearchInput(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			m.moveCursor(-1)

		case "down", "j":
			m.moveCursor(1)

		case "enter", "l", "right":
			m.drillDown()

		case "esc", "h", "left", "backspace":
			m.goBack()

		case "home":
			m.cursor = 0
			m.scrollOff = 0

		case "end":
			m.cursor = m.listLen() - 1
			m.clampScroll()

		case "pgup":
			m.moveCursor(-(m.visibleRows()))

		case "pgdown":
			m.moveCursor(m.visibleRows())

		case "/":
			if m.level != levelDetail {
				m.searching = true
				m.searchText = ""
			}

		case "r":
			// Rescan
			m.scanning = true
			m.scanError = ""
			return m, m.startScan()

		case "d":
			m.initiateDelete()
		}
	}

	return m, nil
}

func (m *Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.searchText = ""
	case "enter":
		m.searching = false
		// filter is already applied live
	case "backspace":
		if len(m.searchText) > 0 {
			m.searchText = m.searchText[:len(m.searchText)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.searchText += msg.String()
		}
	}
	m.cursor = 0
	m.scrollOff = 0
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	n := m.listLen()
	if n == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	m.clampScroll()
}

func (m *Model) clampScroll() {
	visible := m.visibleRows()
	if visible <= 0 {
		return
	}
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+visible {
		m.scrollOff = m.cursor - visible + 1
	}
}

func (m *Model) visibleRows() int {
	// Reserve lines for: title, scan info, breadcrumb, header, separator, bottom help, status
	overhead := 8
	rows := m.height - overhead
	if rows < 3 {
		rows = 3
	}
	return rows
}

func (m *Model) listLen() int {
	switch m.level {
	case levelSanctuaries:
		return len(m.filteredSanctuaries())
	case levelRealms:
		return len(m.filteredRealms())
	case levelSwamps:
		return len(m.filteredSwamps())
	default:
		return 0
	}
}

func (m *Model) filteredSanctuaries() []*explorer.SanctuaryInfo {
	if m.searchText == "" {
		return m.sanctuaries
	}
	var out []*explorer.SanctuaryInfo
	for _, s := range m.sanctuaries {
		if strings.Contains(strings.ToLower(s.Name), strings.ToLower(m.searchText)) {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) filteredRealms() []*explorer.RealmInfo {
	if m.searchText == "" {
		return m.realms
	}
	var out []*explorer.RealmInfo
	for _, r := range m.realms {
		if strings.Contains(strings.ToLower(r.Name), strings.ToLower(m.searchText)) {
			out = append(out, r)
		}
	}
	return out
}

func (m *Model) filteredSwamps() []*explorer.SwampDetail {
	if m.searchText == "" {
		return m.swamps
	}
	var out []*explorer.SwampDetail
	for _, s := range m.swamps {
		if strings.Contains(strings.ToLower(s.Swamp), strings.ToLower(m.searchText)) {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) drillDown() {
	switch m.level {
	case levelSanctuaries:
		items := m.filteredSanctuaries()
		if m.cursor >= len(items) {
			return
		}
		m.selectedSanctuary = items[m.cursor].Name
		m.realms = m.explorer.ListRealms(m.selectedSanctuary)
		m.level = levelRealms
		m.cursor = 0
		m.scrollOff = 0
		m.searchText = ""

	case levelRealms:
		items := m.filteredRealms()
		if m.cursor >= len(items) {
			return
		}
		m.selectedRealm = items[m.cursor].Name
		result := m.explorer.ListSwamps(&explorer.SwampFilter{
			Sanctuary: m.selectedSanctuary,
			Realm:     m.selectedRealm,
			Limit:     10000,
		})
		m.swamps = result.Swamps
		m.level = levelSwamps
		m.cursor = 0
		m.scrollOff = 0
		m.searchText = ""

	case levelSwamps:
		items := m.filteredSwamps()
		if m.cursor >= len(items) {
			return
		}
		m.detail = items[m.cursor]
		m.level = levelDetail
		m.cursor = 0
		m.scrollOff = 0
		m.searchText = ""
	}
}

func (m *Model) goBack() {
	m.searchText = ""
	switch m.level {
	case levelRealms:
		m.level = levelSanctuaries
		m.cursor = 0
		m.scrollOff = 0
	case levelSwamps:
		m.level = levelRealms
		m.cursor = 0
		m.scrollOff = 0
	case levelDetail:
		m.level = levelSwamps
		m.cursor = 0
		m.scrollOff = 0
	}
}

// ── View ────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	var b strings.Builder

	// Title
	b.WriteString("  " + titleStyle.Render(" HydrAIDE Explorer "))
	b.WriteString("  " + labelStyle.Render(m.dataPath))
	b.WriteString("\n")

	// Scan info
	if m.scanning {
		b.WriteString("  " + scanStyle.Render(m.scanInfo) + "\n")
		b.WriteString("\n  " + labelStyle.Render("Press q to quit"))
		return b.String()
	}
	if m.scanError != "" {
		b.WriteString("  " + errorCountStyle.Render("Scan error: "+m.scanError) + "\n")
		return b.String()
	}
	b.WriteString("  " + labelStyle.Render(m.scanInfo) + "\n")

	// Breadcrumb
	b.WriteString("  " + m.renderBreadcrumb() + "\n")
	b.WriteString("\n")

	// Delete error overlay
	if m.deleteError != "" {
		b.WriteString("\n  " + errorCountStyle.Render(m.deleteError) + "\n")
		b.WriteString("  " + labelStyle.Render("Press any key to continue"))
		return b.String()
	}

	// Delete confirmation overlay
	if m.deleteConfirm > 0 {
		b.WriteString(m.renderDeleteConfirm())
		return b.String()
	}

	// Delete in progress
	if m.deleting {
		b.WriteString(m.renderDeleting())
		return b.String()
	}

	// Delete done
	if m.deleteDone {
		b.WriteString(m.renderDeleteDone())
		return b.String()
	}

	// Body
	switch m.level {
	case levelSanctuaries:
		b.WriteString(m.renderSanctuaries())
	case levelRealms:
		b.WriteString(m.renderRealms())
	case levelSwamps:
		b.WriteString(m.renderSwamps())
	case levelDetail:
		b.WriteString(m.renderDetail())
	}

	// Search bar
	if m.searching {
		b.WriteString("\n  " + helpKeyStyle.Render("/") + " " + valueStyle.Render(m.searchText) + cursorStyle.Render("_"))
	}

	// Help line
	b.WriteString("\n" + m.renderHelp())

	return b.String()
}

func (m Model) renderBreadcrumb() string {
	parts := []string{"All"}
	if m.selectedSanctuary != "" && m.level >= levelRealms {
		parts = append(parts, m.selectedSanctuary)
	}
	if m.selectedRealm != "" && m.level >= levelSwamps {
		parts = append(parts, m.selectedRealm)
	}
	if m.detail != nil && m.level == levelDetail {
		parts = append(parts, m.detail.Swamp)
	}
	sep := sepStyle.Render(" / ")
	styled := make([]string, len(parts))
	for i, p := range parts {
		if i == len(parts)-1 {
			styled[i] = breadcrumbStyle.Render(p)
		} else {
			styled[i] = labelStyle.Render(p)
		}
	}
	return strings.Join(styled, sep)
}

func (m Model) renderSanctuaries() string {
	items := m.filteredSanctuaries()
	if len(items) == 0 {
		return "  " + labelStyle.Render("No sanctuaries found.") + "\n"
	}

	nameW, realmW, swampW, sizeW := 28, 10, 10, 12
	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s\n",
		padRight("", 2),
		headerStyle.Render(padRight("SANCTUARY", nameW)),
		headerStyle.Render(padRight("REALMS", realmW)),
		headerStyle.Render(padRight("SWAMPS", swampW)),
		headerStyle.Render(padRight("SIZE", sizeW))))

	b.WriteString("  " + sepStyle.Render(strings.Repeat("─", nameW+realmW+swampW+sizeW+12)) + "\n")

	visible := m.visibleRows()
	end := m.scrollOff + visible
	if end > len(items) {
		end = len(items)
	}

	for i := m.scrollOff; i < end; i++ {
		s := items[i]
		cursor := "  "
		style := rowStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
			style = selectedRowStyle
		}
		b.WriteString(fmt.Sprintf("  %s%s  %s  %s  %s\n",
			cursor,
			style.Render(padRight(s.Name, nameW)),
			valueStyle.Render(padRight(fmt.Sprintf("%d", s.RealmCount), realmW)),
			valueStyle.Render(padRight(fmt.Sprintf("%d", s.SwampCount), swampW)),
			valueStyle.Render(padRight(formatSize(s.TotalSize), sizeW))))
	}

	if len(items) > visible {
		b.WriteString(fmt.Sprintf("  %s\n",
			labelStyle.Render(fmt.Sprintf("  showing %d-%d of %d", m.scrollOff+1, end, len(items)))))
	}

	return b.String()
}

func (m Model) renderRealms() string {
	items := m.filteredRealms()
	if len(items) == 0 {
		return "  " + labelStyle.Render("No realms found.") + "\n"
	}

	nameW, swampW, sizeW := 28, 10, 12
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
		padRight("", 2),
		headerStyle.Render(padRight("REALM", nameW)),
		headerStyle.Render(padRight("SWAMPS", swampW)),
		headerStyle.Render(padRight("SIZE", sizeW))))

	b.WriteString("  " + sepStyle.Render(strings.Repeat("─", nameW+swampW+sizeW+10)) + "\n")

	visible := m.visibleRows()
	end := m.scrollOff + visible
	if end > len(items) {
		end = len(items)
	}

	for i := m.scrollOff; i < end; i++ {
		r := items[i]
		cursor := "  "
		style := rowStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
			style = selectedRowStyle
		}
		b.WriteString(fmt.Sprintf("  %s%s  %s  %s\n",
			cursor,
			style.Render(padRight(r.Name, nameW)),
			valueStyle.Render(padRight(fmt.Sprintf("%d", r.SwampCount), swampW)),
			valueStyle.Render(padRight(formatSize(r.TotalSize), sizeW))))
	}

	if len(items) > visible {
		b.WriteString(fmt.Sprintf("  %s\n",
			labelStyle.Render(fmt.Sprintf("  showing %d-%d of %d", m.scrollOff+1, end, len(items)))))
	}

	return b.String()
}

func (m Model) renderSwamps() string {
	items := m.filteredSwamps()
	if len(items) == 0 {
		return "  " + labelStyle.Render("No swamps found.") + "\n"
	}

	nameW, sizeW, entriesW, verW := 28, 12, 10, 5
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s\n",
		padRight("", 2),
		headerStyle.Render(padRight("SWAMP", nameW)),
		headerStyle.Render(padRight("SIZE", sizeW)),
		headerStyle.Render(padRight("ENTRIES", entriesW)),
		headerStyle.Render(padRight("VER", verW))))

	b.WriteString("  " + sepStyle.Render(strings.Repeat("─", nameW+sizeW+entriesW+verW+12)) + "\n")

	visible := m.visibleRows()
	end := m.scrollOff + visible
	if end > len(items) {
		end = len(items)
	}

	for i := m.scrollOff; i < end; i++ {
		s := items[i]
		cursor := "  "
		style := rowStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
			style = selectedRowStyle
		}
		name := s.Swamp
		if len(name) > nameW-1 {
			name = name[:nameW-4] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s%s  %s  %s  %s\n",
			cursor,
			style.Render(padRight(name, nameW)),
			valueStyle.Render(padRight(formatSize(s.FileSize), sizeW)),
			valueStyle.Render(padRight(fmt.Sprintf("%d", s.EntryCount), entriesW)),
			labelStyle.Render(padRight(fmt.Sprintf("V%d", s.Version), verW))))
	}

	if len(items) > visible {
		b.WriteString(fmt.Sprintf("  %s\n",
			labelStyle.Render(fmt.Sprintf("  showing %d-%d of %d", m.scrollOff+1, end, len(items)))))
	}

	return b.String()
}

func (m Model) renderDetail() string {
	if m.detail == nil {
		return ""
	}
	d := m.detail

	var b strings.Builder

	rows := []struct{ label, value string }{
		{"Name", fmt.Sprintf("%s/%s/%s", d.Sanctuary, d.Realm, d.Swamp)},
		{"File", d.FilePath},
		{"Size", formatSize(d.FileSize)},
		{"Format", fmt.Sprintf("V%d", d.Version)},
		{"Created", d.CreatedAt.Format("2006-01-02 15:04:05")},
		{"Modified", d.ModifiedAt.Format("2006-01-02 15:04:05")},
		{"Entries", fmt.Sprintf("%d", d.EntryCount)},
		{"Blocks", fmt.Sprintf("%d", d.BlockCount)},
		{"Island", d.IslandID},
	}

	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %s  %s\n",
			detailLabelStyle.Render(r.label+":"),
			detailValueStyle.Render(r.value)))
	}

	return b.String()
}

func (m Model) renderHelp() string {
	var pairs []string
	switch m.level {
	case levelSanctuaries, levelRealms, levelSwamps:
		pairs = []string{
			"j/k", "navigate",
			"enter", "drill down",
			"/", "search",
			"r", "rescan",
			"q", "quit",
		}
		if m.level > levelSanctuaries {
			pairs = append([]string{"esc", "back"}, pairs...)
		}
		if m.instanceName != "" {
			pairs = append(pairs, "d", "delete")
		}
	case levelDetail:
		pairs = []string{
			"esc", "back",
			"r", "rescan",
			"q", "quit",
		}
		if m.instanceName != "" {
			pairs = append(pairs, "d", "delete")
		}
	}

	var parts []string
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts,
			helpKeyStyle.Render(pairs[i])+" "+helpDescStyle.Render(pairs[i+1]))
	}
	return "  " + strings.Join(parts, labelStyle.Render("  ·  "))
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func padRight(s string, width int) string {
	if runeLen := lipgloss.Width(s); runeLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}
