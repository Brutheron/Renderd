package reader

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReaderRendersHeaderAndMarkdown(t *testing.T) {
	model := New("# Shipped\n\n- **Tests:** passing\n\n```go\nfmt.Println(\"hello\")\n```", "codex")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := ansi.Strip(updated.(Model).View().Content)

	for _, want := range []string{"FINAL RESPONSE", "CODEX", "Shipped", "Tests", "fmt.Println", "Copy response", "Esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() does not contain %q", want)
		}
	}
}

func TestHeadingsRenderWithoutMarkdownMarkers(t *testing.T) {
	model := New("# Primary\n\n## Secondary\n\n### Tertiary\n\n#### Fourth", "codex")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	rendered := ansi.Strip(updated.(Model).rendered)

	for _, marker := range []string{"# Primary", "## Secondary", "### Tertiary", "#### Fourth"} {
		if strings.Contains(rendered, marker) {
			t.Fatalf("rendered heading still contains Markdown marker %q", marker)
		}
	}
	for _, heading := range []string{"Primary", "Secondary", "Tertiary", "Fourth"} {
		if !strings.Contains(rendered, heading) {
			t.Fatalf("rendered output does not contain heading %q", heading)
		}
	}
}

func TestHeadingHierarchyUsesFullWidthDividers(t *testing.T) {
	model := New("# Primary\n\n## Secondary\n\nBody", "codex")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	rendered := ansi.Strip(updated.(Model).rendered)

	var dividerWidths []int
	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && strings.Trim(trimmed, "─") == "" {
			dividerWidths = append(dividerWidths, ansi.StringWidth(trimmed))
		}
	}
	if len(dividerWidths) != 2 {
		t.Fatalf("rendered %d heading dividers, want 2", len(dividerWidths))
	}
	for index, width := range dividerWidths {
		if width < 60 {
			t.Fatalf("divider %d width = %d, want at least 60", index, width)
		}
	}
}

func TestCopyKeyCopiesWholeMarkdown(t *testing.T) {
	markdown := "# Result\n\n**Everything**, including `Markdown`."
	model := New(markdown, "codex")
	updated, command := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})

	if command == nil {
		t.Fatal("copy key did not return a clipboard command")
	}
	if copied := fmt.Sprint(command()); copied != markdown {
		t.Fatalf("clipboard content = %q, want %q", copied, markdown)
	}
	if !updated.(Model).copied {
		t.Fatal("copy key did not set copied feedback")
	}
}

func TestCopyButtonClickCopiesWholeMarkdown(t *testing.T) {
	model := New("complete response", "codex")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated, command := updated.(Model).Update(tea.MouseClickMsg{X: 3, Y: 23, Button: tea.MouseLeft})

	if command == nil {
		t.Fatal("copy button click did not return a clipboard command")
	}
	if copied := fmt.Sprint(command()); copied != "complete response" {
		t.Fatalf("clipboard content = %q, want complete response", copied)
	}
	if !updated.(Model).copied {
		t.Fatal("copy button click did not set copied feedback")
	}
}

func TestFooterStaysOnOneLineInSidePanel(t *testing.T) {
	model := New("done", "codex")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	view := ansi.Strip(updated.(Model).View().Content)

	if lines := strings.Count(view, "\n") + 1; lines > 20 {
		t.Fatalf("side-panel view rendered %d lines for a 20-line viewport", lines)
	}
}

func TestEscapeQuits(t *testing.T) {
	model := New("done", "codex")
	_, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if command == nil {
		t.Fatal("Escape did not return a quit command")
	}
}
