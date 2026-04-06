package components

import "github.com/charmbracelet/lipgloss"

// Color definitions matching current gocui theme
var (
	// Primary colors
	GreenColor  = lipgloss.Color("120") // ANSI 256 color 120 (bright green)
	WhiteColor  = lipgloss.Color("255") // ANSI 256 color 255 (white)
	GrayColor   = lipgloss.Color("245") // ANSI 256 color 245 (gray)
	BlueColor   = lipgloss.Color("39")  // ANSI 256 color 39 (blue)
	YellowColor = lipgloss.Color("226") // ANSI 256 color 226 (yellow)
	RedColor    = lipgloss.Color("196") // ANSI 256 color 196 (red)
	BlackColor  = lipgloss.Color("0")   // ANSI 256 color 0 (black)

	// UI colors
	BorderColorActive   = GreenColor
	BorderColorInactive = WhiteColor
	TitleColor          = WhiteColor
	SelectedColor       = BlueColor
)

// Base styles
type Styles struct {
	// Panel styles
	ActivePanel   lipgloss.Style
	InactivePanel lipgloss.Style

	// List styles
	ListItem         lipgloss.Style
	ListItemSelected lipgloss.Style
	ListItemGray     lipgloss.Style

	// Status bar styles
	StatusBar       lipgloss.Style
	StatusBarActive lipgloss.Style

	// Main panel styles
	MainPanel       lipgloss.Style
	MainPanelTab    lipgloss.Style
	MainPanelTabSel lipgloss.Style

	// Auth panel styles
	AuthPanel lipgloss.Style
}

// NewStyles creates the default styles with the given terminal size
func NewStyles() Styles {
	return Styles{
		// Active panel has green border
		ActivePanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(BorderColorActive),

		// Inactive panel has white border
		InactivePanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(BorderColorInactive),

		// List item styles
		ListItem:         lipgloss.NewStyle(),
		ListItemSelected: lipgloss.NewStyle().Background(SelectedColor).Foreground(WhiteColor),
		ListItemGray:     lipgloss.NewStyle().Foreground(GrayColor),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Background(lipgloss.Color("8")).
			Foreground(WhiteColor),
		StatusBarActive: lipgloss.NewStyle().
			Background(GreenColor).
			Foreground(BlackColor),

		// Main panel
		MainPanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(BorderColorInactive),
		MainPanelTab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(WhiteColor),
		MainPanelTabSel: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(GreenColor).
			Bold(true),

		// Auth panel
		AuthPanel: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(BorderColorInactive),
	}
}

// SetPanelSize applies width and height to a style
func (s *Styles) SetPanelSize(style lipgloss.Style, width, height int) lipgloss.Style {
	return style.Width(width).Height(height)
}

// PanelTitle returns a styled panel title
func PanelTitle(title string) string {
	return lipgloss.NewStyle().
		Foreground(TitleColor).
		Bold(true).
		Render(" " + title + " ")
}

// WithTitle adds a title to a panel
func WithTitle(content string, title string) string {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(BorderColorInactive).
		Render("\n" + content)
}

// FormatWithGraySuffix formats text with a gray suffix (mimicking current behavior)
func FormatWithGraySuffix(name, suffix string) string {
	if suffix == "" {
		return name
	}
	suffixStyle := lipgloss.NewStyle().Foreground(GrayColor)
	return name + " " + suffixStyle.Render("("+suffix+")")
}
