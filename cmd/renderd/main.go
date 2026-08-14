package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Brutheron/Renderd/internal/agents"
	"github.com/Brutheron/Renderd/internal/agents/claude"
	"github.com/Brutheron/Renderd/internal/agents/codex"
	"github.com/Brutheron/Renderd/internal/herdr"
	"github.com/Brutheron/Renderd/internal/reader"
)

const (
	pluginID       = "brutheron.renderd"
	sourcePaneEnv  = "RENDERD_SOURCE_PANE_ID"
	requestTimeout = 10 * time.Second
)

var refreshRetryDelays = []time.Duration{
	0,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
}

func main() {
	if len(os.Args) != 2 {
		fatal("usage: renderd <open|reader>")
	}

	var err error
	switch os.Args[1] {
	case "open":
		err = openReader()
	case "reader":
		err = runReader()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatal(err.Error())
	}
}

func openReader() error {
	var invocation struct {
		FocusedPaneID string `json:"focused_pane_id"`
	}
	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &invocation); err != nil {
			return fmt.Errorf("decode Herdr plugin context: %w", err)
		}
	}

	paneID := invocation.FocusedPaneID
	if paneID == "" {
		paneID = os.Getenv("HERDR_PANE_ID")
	}
	if paneID == "" {
		return errors.New("Renderd needs a focused Herdr pane")
	}

	herdrBinary := envOr("HERDR_BIN_PATH", "herdr")
	activePluginID := envOr("HERDR_PLUGIN_ID", pluginID)
	command := readerCommand(herdrBinary, activePluginID, paneID)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func readerCommand(herdrBinary, activePluginID, paneID string) *exec.Cmd {
	return exec.Command(
		herdrBinary,
		"plugin", "pane", "open",
		"--plugin", activePluginID,
		"--entrypoint", "reader",
		"--placement", "split",
		"--direction", "right",
		"--target-pane", paneID,
		"--focus",
		"--env", sourcePaneEnv+"="+paneID,
	)
}

func runReader() error {
	paneID := os.Getenv(sourcePaneEnv)
	if paneID == "" {
		return reader.Run(errorDocument(
			"No source pane",
			"Renderd was opened without a Herdr source pane. Close this view and invoke it from a focused agent pane.",
		), "renderd")
	}

	herdrClient := herdr.Client{Binary: envOr("HERDR_BIN_PATH", "herdr")}
	registry := agents.Registry{
		"claude": claude.Client{ConfigDir: os.Getenv("RENDERD_CLAUDE_CONFIG_DIR")},
		"codex":  codex.Client{Binary: envOr("RENDERD_CODEX_BIN", "codex")},
	}

	initialContext, cancelInitial := context.WithTimeout(context.Background(), requestTimeout)
	session, response, err := latestFinalForPane(initialContext, herdrClient, registry, paneID)
	cancelInitial()
	if err != nil {
		agent := session.Agent
		if agent == "" {
			agent = "renderd"
		}
		if errors.Is(err, agents.ErrNoFinalResponse) {
			response = agents.FinalResponse{Agent: agent, Markdown: errorDocument(
				"No completed final response yet",
				fmt.Sprintf("Leave Renderd open. The reader will update when %s finishes its current turn.", displayName(session.Agent)),
			)}
		} else {
			response = agents.FinalResponse{Agent: agent, Markdown: errorDocument("Could not read the agent response", err.Error())}
		}
	}

	liveContext, cancelLive := context.WithCancel(context.Background())
	defer cancelLive()

	events, subscribeErr := herdrClient.SubscribeAgentStatus(liveContext, paneID)
	live := reader.LiveUpdates{
		ConnectionError: subscribeErr,
		Context:         liveContext,
		Events:          events,
		Refresh: func(ctx context.Context, currentTurnID string) (agents.FinalResponse, error) {
			return refreshLatestFinal(ctx, herdrClient, registry, paneID, currentTurnID)
		},
	}
	return reader.RunLive(response, live)
}

// latestFinalForPane resolves the pane's native agent session and reads its
// latest completed response. The session is returned alongside the error so
// callers can label a failure with the agent it came from.
func latestFinalForPane(
	ctx context.Context,
	herdrClient herdr.Client,
	registry agents.Registry,
	paneID string,
) (agents.Session, agents.FinalResponse, error) {
	pane, err := herdrClient.GetPane(ctx, paneID)
	if err != nil {
		return agents.Session{}, agents.FinalResponse{}, err
	}
	if pane.AgentSession == nil || pane.AgentSession.Agent == "" || pane.AgentSession.Value == "" {
		return agents.Session{}, agents.FinalResponse{}, fmt.Errorf(
			"agent session identity unavailable; install Herdr's session integration with `herdr integration install %s` and restart the agent",
			integrationName(pane.Agent),
		)
	}

	session := agents.Session{
		Agent:  pane.AgentSession.Agent,
		Kind:   pane.AgentSession.Kind,
		Source: pane.AgentSession.Source,
		Value:  pane.AgentSession.Value,
	}
	response, err := registry.LatestFinal(ctx, session)
	return session, response, err
}

func refreshLatestFinal(
	ctx context.Context,
	herdrClient herdr.Client,
	registry agents.Registry,
	paneID string,
	currentTurnID string,
) (agents.FinalResponse, error) {
	var latest agents.FinalResponse
	var lastErr error
	for _, delay := range refreshRetryDelays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return agents.FinalResponse{}, ctx.Err()
			case <-timer.C:
			}
		}

		_, response, err := latestFinalForPane(ctx, herdrClient, registry, paneID)
		if err != nil {
			lastErr = err
			continue
		}
		latest = response
		lastErr = nil
		if currentTurnID == "" || response.TurnID != currentTurnID {
			return response, nil
		}
	}
	if latest.Markdown != "" {
		return latest, nil
	}
	if lastErr == nil {
		lastErr = agents.ErrNoFinalResponse
	}
	return agents.FinalResponse{}, lastErr
}

// integrationName is the `herdr integration install` argument for the agent
// Herdr detected in the pane, falling back to a readable placeholder.
func integrationName(agent string) string {
	if agent == "" {
		return "<agent>"
	}
	return agent
}

// displayName titles an agent name for prose shown in the reader.
func displayName(agent string) string {
	if agent == "" {
		return "the agent"
	}
	return strings.ToUpper(agent[:1]) + agent[1:]
}

func errorDocument(title, detail string) string {
	return fmt.Sprintf("# %s\n\n> %s\n\nPress **Esc** to return to the agent terminal.\n", title, strings.TrimSpace(detail))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, "renderd:", message)
	os.Exit(1)
}
