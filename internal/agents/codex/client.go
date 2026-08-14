package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/Brutheron/Renderd/internal/agents"
)

// Client reads persisted Codex threads through the documented App Server
// protocol. It never attaches to or sends input to the interactive Codex TUI.
type Client struct {
	Binary string
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type rpcEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type threadReadResult struct {
	Thread thread `json:"thread"`
}

type thread struct {
	Turns []turn `json:"turns"`
}

type turn struct {
	CompletedAt *int64 `json:"completedAt"`
	ID          string `json:"id"`
	Items       []item `json:"items"`
	Status      string `json:"status"`
}

type item struct {
	ID    string `json:"id"`
	Phase string `json:"phase"`
	Text  string `json:"text"`
	Type  string `json:"type"`
}

// FinalResponses implements agents.Adapter for Codex sessions.
func (c Client) FinalResponses(ctx context.Context, session agents.Session) ([]agents.FinalResponse, error) {
	if session.Agent != "codex" || session.Value == "" {
		return nil, fmt.Errorf("invalid Codex session")
	}

	binary := c.Binary
	if binary == "" {
		binary = "codex"
	}

	cmd := exec.CommandContext(ctx, binary, "app-server")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex App Server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex App Server stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Codex App Server: %w", err)
	}
	defer stopProcess(cmd, stdin)

	encoder := json.NewEncoder(stdin)
	decoder := json.NewDecoder(stdout)

	if err := encoder.Encode(map[string]any{
		"id":     0,
		"method": "initialize",
		"params": map[string]any{
			"clientInfo": map[string]string{
				"name":    "renderd",
				"title":   "Renderd",
				"version": "0.4.0",
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("initialize Codex App Server: %w", err)
	}
	if _, err := readRPCResult(decoder, 0); err != nil {
		return nil, fmt.Errorf("initialize Codex App Server: %w", err)
	}

	if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return nil, fmt.Errorf("acknowledge Codex App Server: %w", err)
	}
	if err := encoder.Encode(map[string]any{
		"id":     1,
		"method": "thread/read",
		"params": map[string]any{
			"includeTurns": true,
			"threadId":     session.Value,
		},
	}); err != nil {
		return nil, fmt.Errorf("request Codex thread: %w", err)
	}

	raw, err := readRPCResult(decoder, 1)
	if err != nil {
		return nil, fmt.Errorf("read Codex thread: %w", err)
	}
	var result threadReadResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode Codex thread: %w", err)
	}

	return selectFinalResponses(result.Thread, session)
}

// LatestFinal keeps the direct client API convenient for live diagnostics.
func (c Client) LatestFinal(ctx context.Context, session agents.Session) (agents.FinalResponse, error) {
	responses, err := c.FinalResponses(ctx, session)
	if err != nil {
		return agents.FinalResponse{}, err
	}
	return responses[len(responses)-1], nil
}

func readRPCResult(decoder *json.Decoder, wantedID int) (json.RawMessage, error) {
	for {
		var envelope rpcEnvelope
		if err := decoder.Decode(&envelope); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("Codex App Server closed before responding")
			}
			return nil, err
		}
		if len(envelope.ID) == 0 {
			continue
		}

		var id int
		if err := json.Unmarshal(envelope.ID, &id); err != nil || id != wantedID {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("Codex App Server error %d: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return envelope.Result, nil
	}
}

func selectLatestFinal(value thread, session agents.Session) (agents.FinalResponse, error) {
	responses, err := selectFinalResponses(value, session)
	if err != nil {
		return agents.FinalResponse{}, err
	}
	return responses[len(responses)-1], nil
}

func selectFinalResponses(value thread, session agents.Session) ([]agents.FinalResponse, error) {
	responses := make([]agents.FinalResponse, 0, len(value.Turns))
	for _, candidate := range value.Turns {
		if candidate.Status != "completed" {
			continue
		}
		for itemIndex := len(candidate.Items) - 1; itemIndex >= 0; itemIndex-- {
			message := candidate.Items[itemIndex]
			if message.Type != "agentMessage" || message.Phase != "final_answer" || message.Text == "" {
				continue
			}

			response := agents.FinalResponse{
				Agent:     "codex",
				Markdown:  message.Text,
				SessionID: session.Value,
				TurnID:    candidate.ID,
			}
			if candidate.CompletedAt != nil {
				response.CompletedAt = time.Unix(*candidate.CompletedAt, 0)
			}
			responses = append(responses, response)
			break
		}
	}

	if len(responses) == 0 {
		return nil, agents.ErrNoFinalResponse
	}
	return responses, nil
}

func stopProcess(cmd *exec.Cmd, stdin io.Closer) {
	_ = stdin.Close()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
