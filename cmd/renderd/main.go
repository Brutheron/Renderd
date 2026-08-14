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
	"github.com/Brutheron/Renderd/internal/agents/codex"
	"github.com/Brutheron/Renderd/internal/herdr"
	"github.com/Brutheron/Renderd/internal/reader"
)

const (
	pluginID       = "brutheron.renderd"
	sourcePaneEnv  = "RENDERD_SOURCE_PANE_ID"
	requestTimeout = 10 * time.Second
)

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

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	pane, err := (herdr.Client{Binary: envOr("HERDR_BIN_PATH", "herdr")}).GetPane(ctx, paneID)
	if err != nil {
		return reader.Run(errorDocument("Could not inspect the Herdr pane", err.Error()), "renderd")
	}
	if pane.AgentSession == nil || pane.AgentSession.Agent != "codex" || pane.AgentSession.Value == "" {
		return reader.Run(errorDocument(
			"Codex session identity unavailable",
			"Install Herdr's Codex integration with `herdr integration install codex`, restart Codex, and try again.",
		), "codex")
	}

	session := agents.Session{
		Agent:  pane.AgentSession.Agent,
		Kind:   pane.AgentSession.Kind,
		Source: pane.AgentSession.Source,
		Value:  pane.AgentSession.Value,
	}
	response, err := (codex.Client{Binary: envOr("RENDERD_CODEX_BIN", "codex")}).LatestFinal(ctx, session)
	if err != nil {
		if errors.Is(err, codex.ErrNoFinalResponse) {
			return reader.Run(errorDocument(
				"No completed final response yet",
				"Let Codex finish its current turn, then open Renderd again.",
			), "codex")
		}
		return reader.Run(errorDocument("Could not read the Codex response", err.Error()), "codex")
	}

	return reader.Run(response.Markdown, response.Agent)
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
