package agents

import (
	"context"
	"slices"
	"strings"
	"testing"
)

type stubAdapter struct {
	markdown string
}

func (s stubAdapter) FinalResponses(context.Context, Session) ([]FinalResponse, error) {
	return []FinalResponse{{Markdown: s.markdown}}, nil
}

func TestRegistryDispatchesToTheSessionAgent(t *testing.T) {
	registry := Registry{
		"claude": stubAdapter{markdown: "claude answer"},
		"codex":  stubAdapter{markdown: "codex answer"},
	}

	response, err := registry.LatestFinal(context.Background(), Session{Agent: "claude", Value: "session-1"})
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown != "claude answer" {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
}

func TestRegistryReturnsSessionHistory(t *testing.T) {
	registry := Registry{"codex": stubHistoryAdapter{}}

	responses, err := registry.FinalResponses(context.Background(), Session{Agent: "codex", Value: "session-1"})
	if err != nil {
		t.Fatalf("FinalResponses() error = %v", err)
	}
	if got := []string{responses[0].Markdown, responses[1].Markdown}; !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("Markdown history = %q", got)
	}
}

type stubHistoryAdapter struct{}

func (stubHistoryAdapter) FinalResponses(context.Context, Session) ([]FinalResponse, error) {
	return []FinalResponse{{Markdown: "one"}, {Markdown: "two"}}, nil
}

func TestRegistryReportsUnsupportedAgents(t *testing.T) {
	registry := Registry{
		"claude": stubAdapter{},
		"codex":  stubAdapter{},
	}

	_, err := registry.LatestFinal(context.Background(), Session{Agent: "droid", Value: "session-1"})
	if err == nil {
		t.Fatal("LatestFinal() accepted an unregistered agent")
	}
	if !strings.Contains(err.Error(), "droid") || !strings.Contains(err.Error(), "claude, codex") {
		t.Fatalf("error = %v, want the agent and the supported list", err)
	}
}

func TestRegistrySupportedIsSorted(t *testing.T) {
	registry := Registry{"codex": stubAdapter{}, "claude": stubAdapter{}}

	if got := registry.Supported(); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Fatalf("Supported() = %q", got)
	}
}
