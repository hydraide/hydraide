package explore

import "github.com/charmbracelet/lipgloss"

var (
	primaryColor   = lipgloss.Color("#7D56F4")
	secondaryColor = lipgloss.Color("#5A9CF7")
	successColor   = lipgloss.Color("#73F59F")
	errorColor     = lipgloss.Color("#FF6B6B")
	warningColor   = lipgloss.Color("#FFE066")
	mutedColor     = lipgloss.Color("#626262")
	whiteColor     = lipgloss.Color("#FFFFFF")
	cyanColor      = lipgloss.Color("#56CCF2")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(whiteColor).
			Background(primaryColor).
			Padding(0, 1)

	breadcrumbStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor).
			Underline(true)

	rowStyle = lipgloss.NewStyle().
			Foreground(whiteColor)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(whiteColor).
				Background(lipgloss.Color("#2d2d3d")).
				Bold(true)

	valueStyle = lipgloss.NewStyle().
			Foreground(cyanColor)

	labelStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	sepStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	cursorStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	scanStyle = lipgloss.NewStyle().
			Foreground(successColor)

	errorCountStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Width(14)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(cyanColor)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1).
			MarginBottom(1)
)
