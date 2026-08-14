# Renderd

Renderd is a reader mode for final responses from AI agents running inside
[Herdr](https://herdr.dev/). The first release targets Codex and renders the
latest completed final response as a focused, scrollable Markdown side panel.

## MVP workflow

1. Focus a Herdr pane running Codex.
2. Press `prefix+m`.
3. Read the latest completed Codex final response in the right-side panel.
4. Press `c` or click **Copy response** to copy the complete Markdown response.
5. Press `esc` to close the panel and return to the untouched Codex terminal.

Renderd reads structured Codex thread data through `codex app-server`. It does
not parse terminal output or send keys to Codex. Clipboard writes use OSC 52,
which Herdr forwards to the host clipboard.

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
