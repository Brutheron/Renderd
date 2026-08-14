package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/Brutheron/Renderd/internal/agents"
)

const subscriptionHandshakeTimeout = 5 * time.Second

// AgentSession is Herdr's read-only native agent session reference.
type AgentSession struct {
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

// Pane contains the focused pane information needed by Renderd.
type Pane struct {
	Agent        string        `json:"agent"`
	AgentStatus  string        `json:"agent_status"`
	AgentSession *AgentSession `json:"agent_session"`
	PaneID       string        `json:"pane_id"`
}

// Client calls the Herdr CLI selected by the plugin host.
type Client struct {
	Binary     string
	SocketPath string
}

// GetPane resolves one pane and its native agent session reference.
func (c Client) GetPane(ctx context.Context, paneID string) (Pane, error) {
	binary := c.Binary
	if binary == "" {
		binary = "herdr"
	}

	output, err := exec.CommandContext(ctx, binary, "pane", "get", paneID).Output()
	if err != nil {
		return Pane{}, fmt.Errorf("read Herdr pane %q: %w", paneID, err)
	}

	var response struct {
		Result struct {
			Pane Pane `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return Pane{}, fmt.Errorf("decode Herdr pane response: %w", err)
	}
	if response.Result.Pane.PaneID == "" {
		return Pane{}, fmt.Errorf("Herdr returned no pane for %q", paneID)
	}

	return response.Result.Pane, nil
}

// SubscribeAgentStatus watches lifecycle changes for one pane over Herdr's
// newline-delimited JSON socket protocol. The returned stream ends after one
// terminal error event or when ctx is canceled.
func (c Client) SubscribeAgentStatus(ctx context.Context, paneID string) (<-chan agents.StatusEvent, error) {
	if paneID == "" {
		return nil, errors.New("Herdr pane ID is required")
	}

	socketPath := c.SocketPath
	if socketPath == "" {
		socketPath = os.Getenv("HERDR_SOCKET_PATH")
	}
	if socketPath == "" {
		return nil, errors.New("HERDR_SOCKET_PATH is unavailable")
	}

	connection, err := (&net.Dialer{Timeout: subscriptionHandshakeTimeout}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to Herdr event socket: %w", err)
	}
	if err := connection.SetDeadline(time.Now().Add(subscriptionHandshakeTimeout)); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("configure Herdr subscription timeout: %w", err)
	}

	encoder := json.NewEncoder(connection)
	decoder := json.NewDecoder(connection)
	request := map[string]any{
		"id":     "renderd_status",
		"method": "events.subscribe",
		"params": map[string]any{
			"subscriptions": []map[string]any{{
				"type":    "pane.agent_status_changed",
				"pane_id": paneID,
			}},
		},
	}
	if err := encoder.Encode(request); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("subscribe to Herdr agent status: %w", err)
	}

	var acknowledgement struct {
		ID     string `json:"id"`
		Result struct {
			Type string `json:"type"`
		} `json:"result"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := decoder.Decode(&acknowledgement); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("read Herdr subscription acknowledgement: %w", err)
	}
	if acknowledgement.Error != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("Herdr subscription error %s: %s", acknowledgement.Error.Code, acknowledgement.Error.Message)
	}
	if acknowledgement.ID != "renderd_status" || acknowledgement.Result.Type != "subscription_started" {
		_ = connection.Close()
		return nil, errors.New("Herdr returned an invalid subscription acknowledgement")
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("clear Herdr subscription timeout: %w", err)
	}

	events := make(chan agents.StatusEvent, 8)
	go readAgentStatusEvents(ctx, connection, decoder, paneID, events)
	return events, nil
}

func readAgentStatusEvents(
	ctx context.Context,
	connection net.Conn,
	decoder *json.Decoder,
	paneID string,
	events chan<- agents.StatusEvent,
) {
	defer close(events)
	defer connection.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-done:
		}
	}()

	for {
		var envelope struct {
			Event string `json:"event"`
			Data  struct {
				AgentStatus string `json:"agent_status"`
				PaneID      string `json:"pane_id"`
			} `json:"data"`
		}
		if err := decoder.Decode(&envelope); err != nil {
			if ctx.Err() == nil {
				if errors.Is(err, io.EOF) {
					err = errors.New("Herdr closed the live event stream")
				}
				select {
				case events <- agents.StatusEvent{Err: fmt.Errorf("read Herdr agent status: %w", err)}:
				case <-ctx.Done():
				}
			}
			return
		}
		if envelope.Event != "pane.agent_status_changed" || envelope.Data.PaneID != paneID {
			continue
		}

		select {
		case events <- agents.StatusEvent{Status: envelope.Data.AgentStatus}:
		case <-ctx.Done():
			return
		}
	}
}
