package codex

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Brutheron/Renderd/internal/agents"
)

// TestLiveLatestFinal is opt-in because it reads a real local Codex session.
// Run it with RENDERD_TEST_CODEX_SESSION=<thread-id> go test ./internal/agents/codex -run TestLive.
func TestLiveLatestFinal(t *testing.T) {
	sessionID := os.Getenv("RENDERD_TEST_CODEX_SESSION")
	if sessionID == "" {
		t.Skip("RENDERD_TEST_CODEX_SESSION is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	response, err := (Client{}).LatestFinal(ctx, agents.Session{
		Agent: "codex",
		Kind:  "id",
		Value: sessionID,
	})
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown == "" {
		t.Fatal("LatestFinal() returned empty Markdown")
	}
	if response.SessionID != sessionID {
		t.Fatalf("SessionID = %q, want %q", response.SessionID, sessionID)
	}
}
