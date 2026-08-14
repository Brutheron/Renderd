# Renderd

Renderd is a reader mode for final responses from AI agents running inside
[Herdr](https://herdr.dev/). The first release targets Codex and renders the
latest completed final response as a focused, scrollable Markdown side panel.

## MVP workflow

1. Focus a Herdr pane running Codex.
2. Press `prefix+m`.
3. Return focus to the Codex pane and continue the conversation. Renderd stays
   open and updates when each new response completes.
4. Press `c` or click **Copy response** to copy the complete Markdown response.
5. Press `r` to refresh manually if the live event connection is unavailable.
6. Focus Renderd and press `esc` when you want to close the panel.

Renderd reads structured Codex thread data through `codex app-server`. It does
not parse terminal output or send keys to Codex. Clipboard writes use OSC 52,
which Herdr forwards to the host clipboard.

Live updates use Herdr's local socket event stream. While Codex is working, the
previous response remains visible. When the source pane reaches `done` or
`idle`, Renderd reads the latest completed turn, compares its turn ID, and
rerenders only when a new final response is available. If the event stream
drops, the document remains readable and the `r` fallback continues to work.

## Development

Requirements:

- Go 1.25 or newer
- Herdr 0.8.0 or newer
- Codex CLI with App Server support

Build and link the local plugin:

```sh
sh scripts/build.sh
herdr plugin link .
```

Install Herdr's Codex session integration if it is not already installed:

```sh
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
