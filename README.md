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

1. Use arrow keys to select a model from `$HOME/.llamabarn/`.
2. Press `p` to focus/unfocus the port input (defaults to 8080).
3. Press `l` to toggle log-to-file (applies on next start).
4. Press `enter` to start `llama-server` for the selected model.
5. Press `s` to stop the server.
6. Press `r` to rescan models.
7. Press `q` to quit.

When log-to-file is enabled, logs are written to:

```
$HOME/.llamabarn/llama-server-logs/YYYYMMDD_HHMMSS.log
```

## Notes

- The TUI uses `-m <model>` and `--port <port>` when invoking `llama-server`.
- If your `llama-server` requires different flags, adapt `main.go` accordingly.
- File logging applies from the next server start (not mid-run).

## License

MIT


