package reader

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Brutheron/Renderd/internal/agents"
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

func TestLiveStatusMovesFromWorkingToRefreshing(t *testing.T) {
	events := make(chan agents.StatusEvent)
	live := &LiveUpdates{
		Context: context.Background(),
		Events:  events,
		Refresh: func(context.Context, string) (agents.FinalResponse, error) {
			return agents.FinalResponse{Agent: "codex", TurnID: "turn-2", Markdown: "new"}, nil
		},
	}
	model := NewLive(agents.FinalResponse{Agent: "codex", TurnID: "turn-1", Markdown: "old"}, live)

	updated, next := model.Update(statusEventMsg{open: true, event: agents.StatusEvent{Status: "working"}})
	model = updated.(Model)
	if next == nil || model.liveLabel() != "WORKING…" {
		t.Fatalf("working state = %q, command nil = %t", model.liveLabel(), next == nil)
	}

	updated, refresh := model.Update(statusEventMsg{open: true, event: agents.StatusEvent{Status: "done"}})
	model = updated.(Model)
	if refresh == nil || !model.refreshing || model.liveLabel() != "REFRESHING…" {
		t.Fatalf("settled state = %q, refreshing = %t", model.liveLabel(), model.refreshing)
	}
}

func TestNewLiveResponseReplacesDocumentAndResetsReaderState(t *testing.T) {
	live := &LiveUpdates{Context: context.Background(), Events: make(chan agents.StatusEvent)}
	model := NewLive(agents.FinalResponse{Agent: "codex", TurnID: "turn-1", Markdown: "old"}, live)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model = updated.(Model)
	model.copied = true

	updated, _ = model.Update(refreshResultMsg{response: agents.FinalResponse{
		Agent: "codex", TurnID: "turn-2", Markdown: "# Fresh response",
	}})
	model = updated.(Model)
	if model.turnID != "turn-2" || model.markdown != "# Fresh response" {
		t.Fatalf("document = turn %q, markdown %q", model.turnID, model.markdown)
	}
	if model.copied {
		t.Fatal("new response retained stale copy confirmation")
	}
	if model.viewport.YOffset() != 0 {
		t.Fatalf("new response offset = %d, want top", model.viewport.YOffset())
	}
	if model.liveLabel() != "UPDATED" {
		t.Fatalf("live label = %q, want UPDATED", model.liveLabel())
	}
}

func TestManualRefreshWorksWhenLiveEventsAreOffline(t *testing.T) {
	live := &LiveUpdates{
		ConnectionError: fmt.Errorf("socket unavailable"),
		Context:         context.Background(),
		Refresh: func(_ context.Context, currentTurnID string) (agents.FinalResponse, error) {
			if currentTurnID != "turn-1" {
				t.Fatalf("current turn ID = %q", currentTurnID)
			}
			return agents.FinalResponse{Agent: "codex", TurnID: "turn-2", Markdown: "new"}, nil
		},
	}
	model := NewLive(agents.FinalResponse{Agent: "codex", TurnID: "turn-1", Markdown: "old"}, live)
	updated, command := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if command == nil || !updated.(Model).refreshing {
		t.Fatal("manual refresh did not start")
	}
	result, ok := command().(refreshResultMsg)
	if !ok || result.err != nil || result.response.TurnID != "turn-2" {
		t.Fatalf("refresh result = %#v", result)
	}
}
