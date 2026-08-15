# Renderd

Renderd is a live-updating Markdown reader for completed Claude Code and Codex
responses inside [Herdr](https://herdr.dev/). It opens session history in a
focused, scrollable side panel and follows new responses without interfering
with the agent's terminal session.

## Install

```sh
herdr plugin install Brutheron/Renderd
```

Complete the one-time agent integration and keybinding setup below.

## How it works

1. Focus a Herdr pane running Claude Code or Codex.
2. Press `prefix+m`.
3. Use `←`/`[` for older responses and `→`/`]` for newer responses. The header
   shows your current position in the session history.
4. Return focus to the agent pane and continue the conversation. Renderd stays
   open and adds each new completed response without interrupting history you
   are reading.
5. Press `c` or click **Copy response** to copy the selected Markdown response.
6. Press `r` to refresh manually if the live event connection is unavailable.
7. Focus Renderd and press `esc` when you want to close the panel.

## Supported agents

| Agent | Herdr integration | Response source |
| --- | --- | --- |
| Claude Code | `claude` | Its structured JSONL session transcript |
| Codex | `codex` | `codex app-server` over JSON-RPC |

Renderd reads structured session data, never terminal output, and never sends
keys to the agent. Herdr's integration identifies the native session belonging
to the source pane, then Renderd dispatches that session to the matching
adapter. Clipboard writes use OSC 52, which Herdr forwards to the host
clipboard.

For Claude Code, Herdr may provide the transcript path directly. If it provides
only a session ID, Renderd looks under
`$CLAUDE_CONFIG_DIR/projects/*/<session>.jsonl`, defaulting to
`~/.claude/projects/*/<session>.jsonl`. If a session moved between project
directories, the newest matching transcript is used.

For Codex, Renderd starts a short-lived `codex app-server`, reads the identified
thread with `includeTurns`, collects its completed `final_answer` items, and
then stops the App Server process. It does not attach to the interactive Codex
TUI.

Live updates use Herdr's local socket event stream. While the agent is working,
the selected response remains visible. When the source pane reaches `done` or
`idle`, Renderd reads the latest completed turn, compares its turn ID, and adds
it to the local history only when it is new. If you are reading an older
response, Renderd shows `NEW RESPONSE` without moving you away from it. If the
event stream drops, the document remains readable and the `r` fallback
continues to work.

### Reading a Claude Code transcript

Claude Code can append several assistant records for one request ID. Renderd
keeps records whose `stop_reason` ends the turn; a record stopped for `tool_use`
is still in progress, and any text in it is narration before a tool call.
Renderd joins final text blocks while skipping thinking blocks and sidechain
records from subagents. The request ID becomes the turn ID used to detect a new
response.

## Supporting another agent

Implement `agents.Adapter` in a new package under `internal/agents`, then
register it by the name Herdr reports for the agent in the registry in
`cmd/renderd/main.go`. Herdr must be able to report a session reference for that
agent — check `herdr integration status`.

## Setup

Requirements:

- Go 1.25 or newer
- Herdr 0.8.0 or newer
- Claude Code and/or the Codex CLI with App Server support

Install the Herdr session integration for each agent you use. You only need the
integration for your chosen agent, though installing both is supported. Without
it, Herdr cannot tell Renderd which session the pane is running.

```sh
# For Claude Code
herdr integration install claude

# For Codex
herdr integration install codex

herdr integration status
```

Restart any already-running agent session after installing its integration.

Add the reader keybinding to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "brutheron.renderd.open"
description = "open response history"
```

Then reload Herdr's configuration:

```sh
herdr server reload-config
```

## Configuration

Most users do not need additional configuration. These environment variables
are available for non-default installations and development:

| Variable | Purpose |
| --- | --- |
| `RENDERD_CLAUDE_CONFIG_DIR` | Override Claude Code's configuration directory for Renderd only |
| `CLAUDE_CONFIG_DIR` | Claude Code configuration directory used when the Renderd-specific override is absent |
| `RENDERD_CODEX_BIN` | Override the `codex` executable used for App Server reads |
| `HERDR_BIN_PATH` | Override the `herdr` executable; normally supplied by the plugin host |

## Troubleshooting

- **Agent session identity unavailable:** install the matching Herdr integration,
  verify it with `herdr integration status`, and restart the agent session.
- **No completed final response yet:** leave Renderd open. It will update when
  the current agent turn finishes.
- **`OFFLINE` or `LIVE UPDATES PAUSED`:** the current document remains usable;
  press `r` to refresh it manually.
- **Claude transcript not found:** check `CLAUDE_CONFIG_DIR`, or set
  `RENDERD_CLAUDE_CONFIG_DIR` to the directory containing `projects/`.
- **Codex App Server error:** confirm that `codex app-server` is available from
  the Codex CLI selected by `RENDERD_CODEX_BIN` or `PATH`.

## Development

Build and link your local checkout:

```sh
sh scripts/build.sh
herdr plugin link .
```

Run the test suite with `go test ./...`.
