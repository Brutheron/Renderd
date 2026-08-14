package reader

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"

	"github.com/Brutheron/Renderd/internal/agents"
)

const (
	maxReadingWidth = 104
	refreshTimeout  = 10 * time.Second
)

// RefreshFunc retrieves the latest structured final response. currentTurnID
// lets the caller distinguish a new document from persistence lag.
type RefreshFunc func(context.Context, string) (agents.FinalResponse, error)

// LiveUpdates configures lifecycle-driven document refreshes.
type LiveUpdates struct {
	ConnectionError error
	Context         context.Context
	Events          <-chan agents.StatusEvent
	Refresh         RefreshFunc
}

type Model struct {
	agent         string
	connected     bool
	copied        bool
	height        int
	live          *LiveUpdates
	markdown      string
	notice        string
	refreshing    bool
	rendered      string
	responseIndex int
	responses     []agents.FinalResponse
	turnID        string
	viewport      viewport.Model
	width         int
	working       bool
}

type statusEventMsg struct {
	event agents.StatusEvent
	open  bool
}

type refreshResultMsg struct {
	response agents.FinalResponse
	err      error
}

// New creates a scrollable Markdown reader.
func New(markdown, agent string) Model {
	return NewLive(agents.FinalResponse{Agent: agent, Markdown: markdown}, nil)
}

// NewLive creates a reader that can follow lifecycle events from its source
// agent while remaining fully usable if the live stream is unavailable.
func NewLive(response agents.FinalResponse, live *LiveUpdates) Model {
	return NewHistory([]agents.FinalResponse{response}, live)
}

// NewHistory creates a reader over a session's completed responses. The newest
// response is selected initially and older responses remain locally browsable.
func NewHistory(responses []agents.FinalResponse, live *LiveUpdates) Model {
	if len(responses) == 0 {
		responses = []agents.FinalResponse{{}}
	}
	sanitized := make([]agents.FinalResponse, len(responses))
	for index, response := range responses {
		sanitized[index] = sanitizeResponse(response)
	}
	latest := len(sanitized) - 1
	model := Model{
		responseIndex: latest,
		responses:     sanitized,
		turnID:        sanitized[latest].TurnID,
		viewport:      viewport.New(viewport.WithWidth(80), viewport.WithHeight(20)),
	}
	model.selectResponse(latest)
	model.live = live
	if live != nil {
		model.connected = live.ConnectionError == nil && live.Events != nil
		if live.ConnectionError != nil {
			model.notice = "LIVE UPDATES PAUSED"
		}
	}
	return model
}

func (m Model) Init() tea.Cmd {
	return waitForStatusEvent(m.live)
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		m.resize()
		return m, nil
	case statusEventMsg:
		if !message.open {
			m.connected = false
			m.notice = "LIVE UPDATES PAUSED"
			return m, nil
		}
		if message.event.Err != nil {
			m.connected = false
			m.notice = "LIVE UPDATES PAUSED"
			return m, nil
		}

		m.connected = true
		nextEvent := waitForStatusEvent(m.live)
		switch message.event.Status {
		case "working":
			m.working = true
			m.notice = ""
			return m, nextEvent
		case "blocked":
			m.working = false
			m.notice = "WAITING FOR INPUT"
			return m, nextEvent
		case "idle", "done":
			m.working = false
			if m.refreshing || m.live == nil || m.live.Refresh == nil {
				return m, nextEvent
			}
			m.refreshing = true
			m.notice = ""
			return m, tea.Batch(nextEvent, refreshDocument(m.live, m.turnID))
		default:
			return m, nextEvent
		}
	case refreshResultMsg:
		m.refreshing = false
		if message.err != nil {
			m.notice = "REFRESH FAILED · PRESS r"
			return m, nil
		}
		if message.response.Markdown == "" || message.response.TurnID != "" && message.response.TurnID == m.turnID {
			m.notice = ""
			return m, nil
		}
		m.appendResponse(message.response)
		return m, nil
	case tea.KeyPressMsg:
		switch message.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m.copied = true
			return m, tea.SetClipboard(m.markdown)
		case "r":
			if m.refreshing || m.live == nil || m.live.Refresh == nil {
				return m, nil
			}
			m.refreshing = true
			m.notice = ""
			return m, refreshDocument(m.live, m.turnID)
		case "left", "[":
			m.navigateHistory(-1)
			return m, nil
		case "right", "]":
			m.navigateHistory(1)
			return m, nil
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
	headerLeft := fmt.Sprintf("FINAL RESPONSE  ·  %s", m.agent)
	headerRight := m.responsePosition()
	if live := m.liveLabel(); live != "" {
		headerRight += "  ·  " + live
	}
	if headerRight != "" && lipgloss.Width(headerLeft)+lipgloss.Width(headerRight)+3 <= m.width {
		headerLeft += strings.Repeat(" ", m.width-lipgloss.Width(headerLeft)-lipgloss.Width(headerRight)-2) + headerRight
	}
	header := headerStyle.Render(headerLeft)

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
		"←/→ history  ·  ↑/↓ scroll  ·  r refresh  ·  Esc close",
		"←/→ history  ·  j/k scroll  ·  Esc close",
		"[/] history  ·  Esc close",
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

func (m Model) responsePosition() string {
	return fmt.Sprintf("%d/%d", m.responseIndex+1, len(m.responses))
}

func (m *Model) navigateHistory(delta int) {
	next := m.responseIndex + delta
	if next < 0 || next >= len(m.responses) {
		return
	}
	m.selectResponse(next)
	m.notice = ""
	m.copied = false
	m.resize()
	m.viewport.GotoTop()
}

func (m *Model) appendResponse(response agents.FinalResponse) {
	response = sanitizeResponse(response)
	wasLatest := m.responseIndex == len(m.responses)-1

	// An initial read error has no turn ID. Replace that temporary document
	// when the first real completed response arrives instead of preserving it
	// as session history.
	if len(m.responses) == 1 && m.responses[0].TurnID == "" && m.live != nil {
		m.responses[0] = response
		m.turnID = response.TurnID
		m.selectResponse(0)
		m.notice = "UPDATED"
		m.copied = false
		m.resize()
		m.viewport.GotoTop()
		return
	}

	m.responses = append(m.responses, response)
	m.turnID = response.TurnID
	if wasLatest {
		m.selectResponse(len(m.responses) - 1)
		m.notice = "UPDATED"
		m.copied = false
		m.resize()
		m.viewport.GotoTop()
		return
	}
	m.notice = "NEW RESPONSE"
}

func (m *Model) selectResponse(index int) {
	m.responseIndex = index
	response := m.responses[index]
	m.agent = strings.ToUpper(response.Agent)
	m.markdown = response.Markdown
}

func sanitizeResponse(response agents.FinalResponse) agents.FinalResponse {
	response.Agent = sanitizeTerminalText(response.Agent)
	response.Markdown = sanitizeTerminalText(response.Markdown)
	return response
}

func (m Model) liveLabel() string {
	if m.live == nil {
		return ""
	}
	if !m.connected {
		return "OFFLINE"
	}
	if m.refreshing {
		return "REFRESHING…"
	}
	if m.working {
		return "WORKING…"
	}
	if m.notice != "" {
		return m.notice
	}
	return "LIVE"
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

// sanitizeTerminalText is the trust boundary between agent-controlled text and
// the terminal renderer. Newlines and tabs are valid Markdown whitespace; all
// other C0/C1 controls are removed before Glamour adds its own trusted styling.
func sanitizeTerminalText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = neutralizeTerminalControlReferences(value)
	return strings.Map(func(character rune) rune {
		switch character {
		case '\n', '\t':
			return character
		case '\r':
			return '\n'
		}
		if isUnsafeTerminalControl(character) {
			return -1
		}
		return character
	}, value)
}

// neutralizeTerminalControlReferences prevents the Markdown renderer from
// recreating a stripped control through numeric entities such as &#27;.
func neutralizeTerminalControlReferences(value string) string {
	var sanitized strings.Builder
	last := 0

	for index := 0; index+3 < len(value); index++ {
		if value[index] != '&' || value[index+1] != '#' {
			continue
		}

		base := 10
		digits := index + 2
		if value[digits] == 'x' || value[digits] == 'X' {
			base = 16
			digits++
		}
		end := digits
		for end < len(value) && numericEntityDigit(value[end], base) {
			end++
		}
		if end == digits || end >= len(value) || value[end] != ';' {
			continue
		}

		codepoint, err := strconv.ParseUint(value[digits:end], base, 32)
		if err != nil || !isUnsafeTerminalControl(rune(codepoint)) {
			continue
		}

		sanitized.WriteString(value[last:index])
		sanitized.WriteRune('\uFFFD')
		last = end + 1
		index = end
	}

	if last == 0 {
		return value
	}
	sanitized.WriteString(value[last:])
	return sanitized.String()
}

func numericEntityDigit(character byte, base int) bool {
	if character >= '0' && character <= '9' {
		return true
	}
	return base == 16 && (character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F')
}

func isUnsafeTerminalControl(character rune) bool {
	return character != '\n' && character != '\t' &&
		(character < 0x20 || character >= 0x7f && character <= 0x9f)
}

func waitForStatusEvent(live *LiveUpdates) tea.Cmd {
	if live == nil || live.Events == nil {
		return nil
	}
	return func() tea.Msg {
		event, open := <-live.Events
		return statusEventMsg{event: event, open: open}
	}
}

func refreshDocument(live *LiveUpdates, currentTurnID string) tea.Cmd {
	return func() tea.Msg {
		parent := live.Context
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, refreshTimeout)
		defer cancel()
		response, err := live.Refresh(ctx, currentTurnID)
		return refreshResultMsg{response: response, err: err}
	}
}

// Run blocks until the user closes the reader.
func Run(markdown, agent string) error {
	_, err := tea.NewProgram(New(markdown, agent)).Run()
	return err
}

// RunLive blocks until the user closes a lifecycle-aware reader.
func RunLive(response agents.FinalResponse, live LiveUpdates) error {
	_, err := tea.NewProgram(NewLive(response, &live)).Run()
	return err
}

// RunHistory blocks until the user closes a lifecycle-aware history reader.
func RunHistory(responses []agents.FinalResponse, live LiveUpdates) error {
	_, err := tea.NewProgram(NewHistory(responses, &live)).Run()
	return err
}
