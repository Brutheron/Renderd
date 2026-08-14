package claude

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Brutheron/Renderd/internal/agents"
)

// TestLiveLatestFinal is opt-in because it reads a real local Claude session.
// Run it with RENDERD_TEST_CLAUDE_SESSION=<session-id-or-transcript-path> go test ./internal/agents/claude -run TestLive.
func TestLiveLatestFinal(t *testing.T) {
	session := os.Getenv("RENDERD_TEST_CLAUDE_SESSION")
	if session == "" {
		t.Skip("RENDERD_TEST_CLAUDE_SESSION is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	response, err := (Client{}).LatestFinal(ctx, agents.Session{
		Agent: "claude",
		Value: session,
	})
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown == "" {
		t.Fatal("LatestFinal() returned empty Markdown")
	}
	if response.TurnID == "" {
		t.Fatal("LatestFinal() returned no turn ID")
	}
}
