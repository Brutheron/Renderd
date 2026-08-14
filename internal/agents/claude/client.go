package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Brutheron/Renderd/internal/agents"
)

const transcriptExtension = ".jsonl"

// assistantMarker is a cheap necessary condition for an assistant record. Lines
// without it cannot be one, whatever key order Claude Code writes.
var assistantMarker = []byte(`"assistant"`)

// finalStopReasons are the API stop reasons that end an assistant turn. A turn
// that stopped for "tool_use" is still running, so any text it carries is
// narration written before a tool call rather than the final response.
var finalStopReasons = map[string]bool{
	"end_turn":      true,
	"max_tokens":    true,
	"stop_sequence": true,
}

// Client reads persisted Claude Code sessions from the structured JSONL
// transcript Claude Code writes for every session. It never attaches to or
// sends input to the interactive Claude TUI.
type Client struct {
	// ConfigDir overrides the Claude Code configuration directory used to
	// resolve a session ID. It defaults to $CLAUDE_CONFIG_DIR, then ~/.claude.
	ConfigDir string
}

// record is the subset of one transcript line Renderd needs. Message stays raw
// because user records carry a plain string there instead of a content list.
type record struct {
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
	RequestID   string          `json:"requestId"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
}

type message struct {
	Content    []block `json:"content"`
	StopReason string  `json:"stop_reason"`
}

type block struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// entry is one completed-turn record reduced to the fields the reader needs.
type entry struct {
	key       string
	sessionID string
	text      string
	timestamp time.Time
	turnID    string
}

// group accumulates the text of one completed turn. Claude Code can write
// several assistant records with the same request ID, so thinking, tool calls,
// and the final text may arrive as separate records for one turn.
type group struct {
	key       string
	sessionID string
	texts     []string
	timestamp time.Time
	turnID    string
}

// LatestFinal implements agents.Adapter for Claude Code sessions.
func (c Client) LatestFinal(ctx context.Context, session agents.Session) (agents.FinalResponse, error) {
	if session.Agent != "claude" || session.Value == "" {
		return agents.FinalResponse{}, errors.New("invalid Claude session")
	}

	path, err := c.transcriptPath(session)
	if err != nil {
		return agents.FinalResponse{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return agents.FinalResponse{}, fmt.Errorf("open Claude transcript: %w", err)
	}
	defer file.Close()

	return selectLatestFinal(ctx, file, session)
}

// transcriptPath resolves the session reference Herdr reported for the pane.
// The Claude integration reports both a session ID and a transcript path, so
// either kind can arrive here.
func (c Client) transcriptPath(session agents.Session) (string, error) {
	if session.Kind == "path" || strings.HasSuffix(session.Value, transcriptExtension) {
		if _, err := os.Stat(session.Value); err != nil {
			return "", fmt.Errorf("open Claude transcript %q: %w", session.Value, err)
		}
		return session.Value, nil
	}
	return c.transcriptForSessionID(session.Value)
}

func (c Client) transcriptForSessionID(sessionID string) (string, error) {
	configDir, err := c.configDir()
	if err != nil {
		return "", err
	}

	projects := filepath.Join(configDir, "projects")
	matches, err := filepath.Glob(filepath.Join(projects, "*", sessionID+transcriptExtension))
	if err != nil {
		return "", fmt.Errorf("search Claude transcripts: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no Claude transcript for session %q under %s", sessionID, projects)
	}
	return newestFile(matches), nil
}

func (c Client) configDir() (string, error) {
	if c.ConfigDir != "" {
		return c.ConfigDir, nil
	}
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate the Claude configuration directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// newestFile picks the most recently written path. The same session ID under
// two project directories means the session moved; the newer file is current.
func newestFile(paths []string) string {
	newest := paths[0]
	var newestTime time.Time
	if info, err := os.Stat(newest); err == nil {
		newestTime = info.ModTime()
	}

	for _, path := range paths[1:] {
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().After(newestTime) {
			continue
		}
		newest = path
		newestTime = info.ModTime()
	}
	return newest
}

// selectLatestFinal streams the transcript and keeps the last completed turn.
// Reading forward costs one pass and holds only one line at a time, which
// matters because transcripts grow past a thousand records.
func selectLatestFinal(ctx context.Context, source io.Reader, session agents.Session) (agents.FinalResponse, error) {
	reader := bufio.NewReader(source)
	var latest group

	for {
		if err := ctx.Err(); err != nil {
			return agents.FinalResponse{}, err
		}

		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if completed, ok := decodeFinalEntry(line); ok {
				latest = latest.add(completed)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return agents.FinalResponse{}, fmt.Errorf("read Claude transcript: %w", readErr)
		}
	}

	if len(latest.texts) == 0 {
		return agents.FinalResponse{}, agents.ErrNoFinalResponse
	}

	sessionID := latest.sessionID
	if sessionID == "" {
		sessionID = session.Value
	}
	return agents.FinalResponse{
		Agent:       "claude",
		CompletedAt: latest.timestamp,
		Markdown:    strings.Join(latest.texts, "\n\n"),
		SessionID:   sessionID,
		TurnID:      latest.turnID,
	}, nil
}

// add starts a new turn when the request ID changes, so the accumulated text
// always belongs to the newest completed turn seen so far.
func (g group) add(completed entry) group {
	if completed.key != g.key {
		g = group{key: completed.key, turnID: completed.turnID}
	}
	if completed.text != "" {
		g.texts = append(g.texts, completed.text)
	}
	if completed.sessionID != "" {
		g.sessionID = completed.sessionID
	}
	if !completed.timestamp.IsZero() {
		g.timestamp = completed.timestamp
	}
	return g
}

// decodeFinalEntry reports whether one transcript line belongs to a completed
// assistant turn. Unparseable lines are skipped rather than failing the read:
// the final line is routinely half-written while Claude is still streaming.
func decodeFinalEntry(line []byte) (entry, bool) {
	if !bytes.Contains(line, assistantMarker) {
		return entry{}, false
	}

	var raw record
	if err := json.Unmarshal(line, &raw); err != nil {
		return entry{}, false
	}
	// Sidechain records are subagent conversations, not the pane's own answer.
	if raw.Type != "assistant" || raw.IsSidechain || len(raw.Message) == 0 {
		return entry{}, false
	}

	var body message
	if err := json.Unmarshal(raw.Message, &body); err != nil {
		return entry{}, false
	}
	if !finalStopReasons[body.StopReason] {
		return entry{}, false
	}

	completed := entry{key: raw.RequestID, sessionID: raw.SessionID, turnID: raw.RequestID}
	if completed.key == "" {
		completed.key = raw.UUID
		completed.turnID = raw.UUID
	}

	var texts []string
	for _, content := range body.Content {
		if content.Type != "text" || strings.TrimSpace(content.Text) == "" {
			continue
		}
		texts = append(texts, strings.TrimRight(content.Text, "\n"))
	}
	completed.text = strings.Join(texts, "\n\n")

	if stamp, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
		completed.timestamp = stamp
	}
	return completed, true
}
