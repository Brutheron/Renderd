package reader

import (
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

const maxReadingWidth = 104

type Model struct {
	agent    string
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
	footerText := fmt.Sprintf("  ↑/↓ or j/k scroll  ·  PgUp/PgDn page  ·  Esc close%6d%%  ", percent)
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#94A3B8")).
		Background(lipgloss.Color("#111827")).
		Width(m.width).
		Render(footerText)

	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), footer))
	view.AltScreen = true
	return view
}

func (m *Model) resize() {
	viewportHeight := max(1, m.height-2)
	readingWidth := min(maxReadingWidth, max(40, m.width-4))
	gutter := max(0, (m.width-readingWidth)/2)
	offset := m.viewport.YOffset()

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.TokyoNightStyle),
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

// Run blocks until the user closes the reader.
func Run(markdown, agent string) error {
	_, err := tea.NewProgram(New(markdown, agent)).Run()
	return err
}
