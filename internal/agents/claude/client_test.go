package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Brutheron/Renderd/internal/agents"
)

// transcript lines mirror the records Claude Code appends while a session runs:
// one line per content block, several lines sharing one request ID per turn.
const (
	userPrompt      = `{"type":"user","sessionId":"session-1","message":{"role":"user","content":"summarise this"}}`
	toolResult      = `{"type":"user","sessionId":"session-1","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`
	narrationRecord = `{"type":"assistant","sessionId":"session-1","requestId":"req-running","uuid":"uuid-narration","timestamp":"2026-08-14T10:00:00.000Z","isSidechain":false,"message":{"role":"assistant","stop_reason":"tool_use","content":[{"type":"text","text":"Let me check the config."}]}}`
	toolUseRecord   = `{"type":"assistant","sessionId":"session-1","requestId":"req-running","uuid":"uuid-tool","timestamp":"2026-08-14T10:00:01.000Z","isSidechain":false,"message":{"role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use","name":"Read","input":{}}]}}`
	sidechainRecord = `{"type":"assistant","sessionId":"session-1","requestId":"req-subagent","uuid":"uuid-sub","timestamp":"2026-08-14T10:00:02.000Z","isSidechain":true,"message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"Subagent report."}]}}`
	thinkingRecord  = `{"type":"assistant","sessionId":"session-1","requestId":"req-final","uuid":"uuid-thinking","timestamp":"2026-08-14T10:00:03.000Z","isSidechain":false,"message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"thinking","thinking":"weighing options"}]}}`
	finalRecord     = `{"type":"assistant","sessionId":"session-1","requestId":"req-final","uuid":"uuid-final","timestamp":"2026-08-14T10:00:04.000Z","isSidechain":false,"message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"# Done\n\nThe answer."}]}}`
	olderFinal      = `{"type":"assistant","sessionId":"session-1","requestId":"req-old","uuid":"uuid-old","timestamp":"2026-08-14T09:00:00.000Z","isSidechain":false,"message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"Older answer."}]}}`
)

func transcript(t *testing.T, lines ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "session-1.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}

func readTranscript(t *testing.T, path string) (agents.FinalResponse, error) {
	t.Helper()

	return Client{}.LatestFinal(context.Background(), agents.Session{
		Agent: "claude",
		Kind:  "path",
		Value: path,
	})
}

func TestLatestFinalJoinsTurnRecordsAndSkipsNonFinalContent(t *testing.T) {
	path := transcript(t,
		userPrompt,
		narrationRecord,
		toolUseRecord,
		toolResult,
		sidechainRecord,
		thinkingRecord,
		finalRecord,
	)

	response, err := readTranscript(t, path)
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown != "# Done\n\nThe answer." {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
	if response.TurnID != "req-final" {
		t.Fatalf("TurnID = %q, want the request ID of the completed turn", response.TurnID)
	}
	if response.Agent != "claude" || response.SessionID != "session-1" {
		t.Fatalf("Agent = %q, SessionID = %q", response.Agent, response.SessionID)
	}
	if response.CompletedAt.UTC().Format("15:04:05") != "10:00:04" {
		t.Fatalf("CompletedAt = %v, want the timestamp of the last record in the turn", response.CompletedAt)
	}
}

func TestLatestFinalChoosesNewestCompletedTurn(t *testing.T) {
	path := transcript(t, olderFinal, userPrompt, thinkingRecord, finalRecord)

	response, err := readTranscript(t, path)
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown != "# Done\n\nThe answer." {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
}

// A turn in flight must not blank the reader: the previous answer stays until
// the running turn completes.
func TestLatestFinalKeepsPreviousAnswerWhileTurnRuns(t *testing.T) {
	path := transcript(t, olderFinal, userPrompt, narrationRecord, toolUseRecord)

	response, err := readTranscript(t, path)
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown != "Older answer." {
		t.Fatalf("Markdown = %q, want the previous completed turn", response.Markdown)
	}
	if response.TurnID != "req-old" {
		t.Fatalf("TurnID = %q", response.TurnID)
	}
}

func TestLatestFinalReturnsNotFound(t *testing.T) {
	path := transcript(t, userPrompt, narrationRecord, toolUseRecord, sidechainRecord)

	_, err := readTranscript(t, path)
	if !errors.Is(err, agents.ErrNoFinalResponse) {
		t.Fatalf("error = %v, want ErrNoFinalResponse", err)
	}
}

// Claude appends to the transcript while it streams, so the last line can be
// half-written when the reader opens the file.
func TestLatestFinalSkipsPartiallyWrittenLine(t *testing.T) {
	path := transcript(t, finalRecord, `{"type":"assistant","sessionId":"sess`)

	response, err := readTranscript(t, path)
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown != "# Done\n\nThe answer." {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
}

func TestLatestFinalResolvesSessionIDThroughProjectsDirectory(t *testing.T) {
	configDir := t.TempDir()
	projectDir := filepath.Join(configDir, "projects", "-Users-someone-Developer-Renderd")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "session-1.jsonl"), []byte(finalRecord+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	response, err := Client{ConfigDir: configDir}.LatestFinal(context.Background(), agents.Session{
		Agent: "claude",
		Kind:  "id",
		Value: "session-1",
	})
	if err != nil {
		t.Fatalf("LatestFinal() error = %v", err)
	}
	if response.Markdown != "# Done\n\nThe answer." {
		t.Fatalf("Markdown = %q", response.Markdown)
	}
}

func TestLatestFinalReportsMissingTranscript(t *testing.T) {
	_, err := Client{ConfigDir: t.TempDir()}.LatestFinal(context.Background(), agents.Session{
		Agent: "claude",
		Kind:  "id",
		Value: "session-missing",
	})
	if err == nil || !strings.Contains(err.Error(), "session-missing") {
		t.Fatalf("error = %v, want a message naming the session", err)
	}
}

func TestLatestFinalRejectsForeignSession(t *testing.T) {
	if _, err := (Client{}).LatestFinal(context.Background(), agents.Session{Agent: "codex", Value: "x"}); err == nil {
		t.Fatal("LatestFinal() accepted a non-Claude session")
	}
}
