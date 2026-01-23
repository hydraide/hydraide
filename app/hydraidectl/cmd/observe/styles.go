package observe

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	primaryColor   = lipgloss.Color("#7D56F4") // Purple
	secondaryColor = lipgloss.Color("#5A9CF7") // Blue
	successColor   = lipgloss.Color("#73F59F") // Green
	errorColor     = lipgloss.Color("#FF6B6B") // Red
	warningColor   = lipgloss.Color("#FFE066") // Yellow
	mutedColor     = lipgloss.Color("#626262") // Gray
	bgColor        = lipgloss.Color("#1a1b26") // Dark background
)

// Styles
var (
	// Title bar style
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
			Padding(0, 1)

	// Tab styles
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(secondaryColor).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Padding(0, 2)

	// Panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(0, 1)

	activePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 1)

	// Event row styles
	eventRowStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#2d2d3d")).
				Padding(0, 1)

	// Method styles
	methodGetStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(successColor).
			Width(8)

	methodSetStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor).
			Width(8)

	methodDeleteStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(errorColor).
				Width(8)

	methodOtherStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(warningColor).
				Width(8)

	// Status styles
	successStyle = lipgloss.NewStyle().
			Foreground(successColor)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	// Timestamp style
	timestampStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(12)

	// Duration style
	durationStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Width(8).
			Align(lipgloss.Right)

	// Swamp name style
	swampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	// Stats panel styles
	statLabelStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	statValueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF"))

	// Error details styles
	errorDetailHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(errorColor)

	errorDetailLabelStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Width(12)

	errorDetailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF"))

	stackTraceStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginLeft(2)

	// Help style
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Paused indicator
	pausedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(warningColor).
			Background(lipgloss.Color("#3d3d00")).
			Padding(0, 1)

	// Replay mode indicator
	replayStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(lipgloss.Color("#2d2d4d")).
			Padding(0, 1)

	// Key bindings style
	keyStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	keyDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1e1e2e")).
			Padding(0, 1)
)

// getMethodStyle returns the appropriate style for a method name.
func getMethodStyle(method string) lipgloss.Style {
	switch method {
	case "Get", "GetAll", "GetByIndex", "GetByKeys":
		return methodGetStyle
	case "Set":
		return methodSetStyle
	case "Delete", "Destroy":
		return methodDeleteStyle
	default:
		return methodOtherStyle
	}
}
