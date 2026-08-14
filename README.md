# Renderd

Renderd is a live-updating Markdown reader for final responses from AI agents
running inside [Herdr](https://herdr.dev/). It opens the latest completed
response in a focused, scrollable side panel and follows new responses without
interfering with the agent's terminal session.

## How it works

1. Focus a Herdr pane running Claude Code or Codex.
2. Press `prefix+m`.
3. Return focus to the agent pane and continue the conversation. Renderd stays
   open and updates when each new response completes.
4. Press `c` or click **Copy response** to copy the complete Markdown response.
5. Press `r` to refresh manually if the live event connection is unavailable.
6. Focus Renderd and press `esc` when you want to close the panel.

Renderd reads structured session data, never terminal output, and never sends
keys to the agent. Herdr's session integrations report which native session
belongs to a pane, and one adapter per agent family turns that reference into a
document:

| Agent | Source |
| --- | --- |
| Claude Code | the session transcript at `~/.claude/projects/<project>/<session>.jsonl` |
| Codex | `codex app-server` over the documented JSON-RPC protocol |

Clipboard writes use OSC 52, which Herdr forwards to the host clipboard.

Live updates use Herdr's local socket event stream. While the agent is working,
the previous response remains visible. When the source pane reaches `done` or
`idle`, Renderd reads the latest completed turn, compares its turn ID, and
rerenders only when a new final response is available. If the event stream
drops, the document remains readable and the `r` fallback continues to work.

### Reading a Claude Code transcript

Claude Code appends one JSON record per content block, so a single turn spans
several consecutive records that share a request ID. Renderd keeps the records
whose `stop_reason` ends the turn — a turn that stopped for `tool_use` is still
running, and its text is narration written before a tool call — and joins their
text blocks. Thinking blocks and sidechain records, which belong to subagents
rather than the pane's own conversation, are skipped. The request ID becomes the
turn ID that drives rerendering.

## Supporting another agent

Implement `agents.Adapter` in a new package under `internal/agents`, then
register it by the name Herdr reports for the agent in the registry in
`cmd/renderd/main.go`. Herdr must be able to report a session reference for that
agent — check `herdr integration status`.

## Development

Requirements:

- Go 1.25 or newer
- Herdr 0.8.0 or newer
- Claude Code, or the Codex CLI with App Server support

Build and link the local plugin:

```sh
sh scripts/build.sh
herdr plugin link .
```

Install Herdr's session integration for each agent you use. Renderd depends on
it: without one, Herdr cannot tell the plugin which session a pane is running.

```sh
herdr integration install claude
herdr integration install codex
```

Add the reader keybinding to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "brutheron.renderd.open"
description = "open latest final response"
```

Then reload Herdr's configuration:

```sh
herdr server reload-config
```

Run the test suite with `go test ./...`.
