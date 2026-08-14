package reader

import (
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

const maxReadingWidth = 104

type Model struct {
	agent    string
	copied   bool
	height   int
	markdown string
	rendered string
	viewport viewport.Model
	width    int
}

// New creates a scrollable Markdown reader.
func New(markdown, agent string) Model {
	return Model{
		agent:    strings.ToUpper(agent),
		markdown: markdown,
		viewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(20)),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		m.resize()
		return m, nil
	case tea.KeyPressMsg:
		switch message.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m.copied = true
			return m, tea.SetClipboard(m.markdown)
		case "j", "down":
			m.viewport.ScrollDown(1)
			return m, nil
		case "k", "up":
			m.viewport.ScrollUp(1)
			return m, nil
		case "pgdown", "space":
			m.viewport.PageDown()
			return m, nil
		case "pgup":
			m.viewport.PageUp()
			return m, nil
		case "home", "g":
			m.viewport.GotoTop()
			return m, nil
		case "end", "G":
			m.viewport.GotoBottom()
			return m, nil
		}
	case tea.MouseClickMsg:
		if message.Button == tea.MouseLeft && m.copyButtonHit(message.X, message.Y) {
			m.copied = true
			return m, tea.SetClipboard(m.markdown)
		}
	}

	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		view := tea.NewView("Loading final response…")
		view.AltScreen = true
		return view
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#F8FAFC")).
		Background(lipgloss.Color("#635BFF")).
		Padding(0, 1).
		Width(m.width)
	header := headerStyle.Render(fmt.Sprintf("FINAL RESPONSE  ·  %s", m.agent))

	percent := int(math.Round(m.viewport.ScrollPercent() * 100))
	buttonLabel := m.copyButtonLabel()
	buttonBackground := lipgloss.Color("#2563EB")
	if m.copied {
		buttonBackground = lipgloss.Color("#15803D")
	}
	button := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#F8FAFC")).
		Background(buttonBackground).
		Render(buttonLabel)
	left := " " + button
	right := fmt.Sprintf("%3d%% ", percent)
	if lipgloss.Width(left)+lipgloss.Width(right) > m.width {
		right = ""
	}
	for _, hint := range []string{
		"↑/↓ scroll  ·  PgUp/PgDn page  ·  Esc close",
		"j/k scroll  ·  Esc close",
		"Esc close",
	} {
		if lipgloss.Width(left)+2+lipgloss.Width(hint)+lipgloss.Width(right) <= m.width {
			left += "  " + hint
			break
		}
	}
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right)))
	footerText := left + spacer + right
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Background(lipgloss.Color("#111827")).
		Width(m.width).
		Render(footerText)

	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), footer))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) copyButtonHit(x, y int) bool {
	if m.height == 0 || y != m.height-1 {
		return false
	}
	return x >= 1 && x < 1+lipgloss.Width(m.copyButtonLabel())
}

func (m Model) copyButtonLabel() string {
	if m.copied {
		return " ✓  Copied "
	}
	if m.width > 0 && m.width < 36 {
		return " c  Copy "
	}
	return " c  Copy response "
}

func (m *Model) resize() {
	viewportHeight := max(1, m.height-2)
	readingWidth := min(maxReadingWidth, max(20, m.width-4))
	gutter := max(0, (m.width-readingWidth)/2)
	offset := m.viewport.YOffset()

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(renderStyle(readingWidth)),
		glamour.WithWordWrap(readingWidth),
	)
	if err != nil {
		m.rendered = m.markdown
	} else if rendered, renderErr := renderer.Render(m.markdown); renderErr != nil {
		m.rendered = m.markdown
	} else {
		m.rendered = rendered
	}

	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(viewportHeight)
	m.viewport.Style = lipgloss.NewStyle().PaddingLeft(gutter).PaddingRight(gutter)
	m.viewport.SetContent(m.rendered)
	m.viewport.SetYOffset(offset)
}

func renderStyle(readingWidth int) ansi.StyleConfig {
	style := styles.TokyoNightStyleConfig
	style.H1.Prefix = ""
	style.H1.Suffix = "\n" + strings.Repeat("─", max(1, readingWidth-4))
	style.H1.Color = pointer("#c099ff")
	style.H1.Bold = pointer(true)
	style.H1.Underline = pointer(false)
	style.H2.Prefix = ""
	style.H2.Suffix = "\n" + strings.Repeat("─", max(1, readingWidth-4))
	style.H2.Color = pointer("#7aa2f7")
	style.H2.Bold = pointer(true)
	style.H3.Prefix = ""
	style.H3.Color = pointer("#2ac3de")
	style.H3.Bold = pointer(true)
	style.H4.Prefix = ""
	style.H4.Color = pointer("#9ece6a")
	style.H4.Bold = pointer(true)
	style.H5.Prefix = ""
	style.H5.Color = pointer("#e0af68")
	style.H6.Prefix = ""
	style.H6.Color = pointer("#94a3b8")
	style.H6.Faint = pointer(true)
	return style
}

func pointer[T any](value T) *T {
	return &value
}

// Run blocks until the user closes the reader.
func Run(markdown, agent string) error {
	_, err := tea.NewProgram(New(markdown, agent)).Run()
	return err
}
