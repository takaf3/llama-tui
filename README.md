# llama-tui

Fancy terminal UI wrapper for `llama-server` (OpenAI-compatible server for GGUF models).

Built with Charmbracelet's Bubble Tea, Bubbles, and Lip Gloss libraries for a delightful TUI experience. See Charmbracelet projects: `https://github.com/charmbracelet`.

## Features

- Lists `.gguf` models under `$HOME/.llamabarn/` (recursively)
- Starts `llama-server` with the selected model and chosen port
- Streams server logs live in the UI
- Optional log file output to `$HOME/.llamabarn/llama-server-logs/`

## Requirements

- Go 1.22+ installed
- `llama-server` on your PATH (from llama.cpp or your distribution)
- Models stored at `$HOME/.llamabarn/` (e.g. `.../mistral-7b.Q4_K_M.gguf`)

## Install & Run

### Using Make (Recommended)

Build and install to `$HOME/.local/bin`:

```bash
make build && make install
```

Make sure `$HOME/.local/bin` is in your PATH. For zsh, add to `~/.zshrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload your shell or run `source ~/.zshrc`.

To uninstall:

```bash
make uninstall
```

### Manual Installation

Build manually:

```bash
go build -o llama-tui
./llama-tui
```

Or run directly without building:

```bash
go run .
```

### Other Make Targets

- `make build` - Build the binary to `./bin/llama-tui`
- `make install` - Install to `$HOME/.local/bin` (or override with `INSTALL_DIR`)
- `make uninstall` - Remove from `$HOME/.local/bin`
- `make run` - Run directly with `go run .`
- `make clean` - Remove the `./bin` directory

## Usage

### Keyboard Shortcuts

- `[enter]` - Start server with selected model
- `[s]` - Stop the running server (shows "Stopping..." status until confirmed)
- `[r]` - Refresh/rescan models list
- `[p]` - Focus/unfocus port input (defaults to 8080)
- `[l]` - Toggle file logging (applies on next start)
- `[h]` - Toggle help overlay
- `[q]` or `[ctrl+c]` - Quit (automatically stops server if running)

### Status Indicators

The header shows the current server status:
- `[RUNNING]` - Server is active and serving requests
- `[STOPPING]` - Server shutdown in progress (wait for confirmation)
- `[STOPPED]` - No server running

### Workflow

1. Use arrow keys to select a model from `$HOME/.llamabarn/`.
2. Press `p` to focus the port input and type a port number (defaults to 8080).
3. Press `l` to toggle log-to-file if desired (applies on next start).
4. Press `enter` to start `llama-server` for the selected model.
5. Monitor logs in the right panel. The status will show `[RUNNING]` when active.
6. Press `s` to stop the server. The status will change to `[STOPPING]` and then `[STOPPED]` when complete.
7. Press `h` anytime to view the help overlay with all shortcuts.
8. Press `q` to quit (server will be stopped automatically if running).

When log-to-file is enabled, logs are written to:

```
$HOME/.llamabarn/llama-server-logs/YYYYMMDD_HHMMSS.log
```

## Features & Behavior

### Reliable Stop Operation

When you press `[s]` to stop the server:
- The status immediately changes to `[STOPPING]` with a clear message
- A log message appears: `[ui] Stopping server...`
- The UI waits for the server process to actually exit before showing `[STOPPED]`
- A confirmation message appears: `[ui] Server stopped successfully`
- This ensures you always know when the server has fully stopped

### Contextual Actions

- Actions are disabled when inappropriate (e.g., can't refresh while server is running)
- Clear status messages explain what's happening or why an action can't be performed
- Dynamic help line shows relevant shortcuts based on current state
- Press `[h]` to view a comprehensive help overlay anytime

### Status Feedback

All actions provide clear feedback:
- Starting server: Shows "Starting..." status and logs initial message
- Stopping server: Shows "Stopping..." status until confirmed stopped
- Port focus: Shows "Port input focused/unfocused"
- Toggle logging: Shows "Log to file: enabled/disabled"
- Refresh: Shows "Scanning for models..." and result count

## Notes

- The TUI uses `-m <model>`, `--port <port>`, and `--jinja` when invoking `llama-server`.
- The `--jinja` flag is enabled by default to support OpenAI Tools/function calling. If your `llama-server` doesn't recognize `--jinja`, update to a newer `llama.cpp` build.
- If your `llama-server` requires different flags, adapt `main.go` accordingly.
- File logging applies from the next server start (not mid-run).
- When quitting with `[q]` while server is running, the app waits for the server to stop before exiting.

## License

MIT


