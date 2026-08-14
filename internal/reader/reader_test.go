package reader

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReaderRendersHeaderAndMarkdown(t *testing.T) {
	model := New("# Shipped\n\n- **Tests:** passing\n\n```go\nfmt.Println(\"hello\")\n```", "codex")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := ansi.Strip(updated.(Model).View().Content)

	for _, want := range []string{"FINAL RESPONSE", "CODEX", "Shipped", "Tests", "fmt.Println", "Esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() does not contain %q", want)
		}
	}
}

func TestEscapeQuits(t *testing.T) {
	model := New("done", "codex")
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if command == nil {
		t.Fatal("Escape did not return a quit command")
	}
}
