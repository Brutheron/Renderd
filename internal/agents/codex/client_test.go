package codex

import (
	"errors"
	"testing"

	"github.com/Brutheron/Renderd/internal/agents"
)

func TestSelectLatestFinalIgnoresPartialAndNonFinalItems(t *testing.T) {
	completedAt := int64(1_700_000_000)
	value := thread{Turns: []turn{
		{
			ID:          "turn-old",
			Status:      "completed",
			CompletedAt: &completedAt,
			Items: []item{
				{Type: "agentMessage", Phase: "commentary", Text: "working"},
				{Type: "commandExecution"},
				{Type: "agentMessage", Phase: "final_answer", Text: "# Finished\n\nOld answer."},
			},
		},
		{
			ID:     "turn-active",
			Status: "interrupted",
			Items: []item{
				{Type: "agentMessage", Phase: "commentary", Text: "still working"},
			},
		},
	}}

	response, err := selectLatestFinal(value, agents.Session{Agent: "codex", Value: "session-1"})
	if err != nil {
		t.Fatalf("selectLatestFinal() error = %v", err)
	}
	if response.Markdown != "# Finished\n\nOld answer." {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
	if response.TurnID != "turn-old" {
		t.Fatalf("TurnID = %q", response.TurnID)
	}
	if response.CompletedAt.Unix() != completedAt {
		t.Fatalf("CompletedAt = %v", response.CompletedAt)
	}
}

func TestSelectLatestFinalChoosesNewestCompletedTurn(t *testing.T) {
	value := thread{Turns: []turn{
		{ID: "one", Status: "completed", Items: []item{{Type: "agentMessage", Phase: "final_answer", Text: "one"}}},
		{ID: "two", Status: "completed", Items: []item{{Type: "agentMessage", Phase: "final_answer", Text: "two"}}},
	}}

	response, err := selectLatestFinal(value, agents.Session{Agent: "codex", Value: "session-1"})
	if err != nil {
		t.Fatalf("selectLatestFinal() error = %v", err)
	}
	if response.Markdown != "two" {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
}

func TestSelectLatestFinalReturnsNotFound(t *testing.T) {
	value := thread{Turns: []turn{{
		ID:     "turn-active",
		Status: "inProgress",
		Items:  []item{{Type: "agentMessage", Phase: "commentary", Text: "working"}},
	}}}

	_, err := selectLatestFinal(value, agents.Session{Agent: "codex", Value: "session-1"})
	if !errors.Is(err, agents.ErrNoFinalResponse) {
		t.Fatalf("error = %v, want ErrNoFinalResponse", err)
	}
}
