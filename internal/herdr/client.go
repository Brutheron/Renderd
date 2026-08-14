package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

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
	AgentSession *AgentSession `json:"agent_session"`
	PaneID       string        `json:"pane_id"`
}

// Client calls the Herdr CLI selected by the plugin host.
type Client struct {
	Binary string
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
