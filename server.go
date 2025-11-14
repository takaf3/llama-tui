package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// validatePort checks if the port string is a valid port number (1-65535)
func validatePort(portStr string) (int, error) {
	if portStr == "" {
		return 0, fmt.Errorf("port cannot be empty")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("port must be a number")
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return port, nil
}

// getLlamaServerBinary resolves the llama-server executable path.
// Priority:
// 1) LLAMA_SERVER_BIN environment variable (absolute path)
// 2) Look up "llama-server" in PATH
func getLlamaServerBinary() (string, error) {
	if envPath := strings.TrimSpace(os.Getenv("LLAMA_SERVER_BIN")); envPath != "" {
		if info, err := os.Stat(envPath); err == nil && !info.IsDir() {
			return envPath, nil
		}
		return "", fmt.Errorf("LLAMA_SERVER_BIN points to an invalid path: %q", envPath)
	}
	bin, err := exec.LookPath("llama-server")
	if err != nil {
		return "", fmt.Errorf("llama-server not found in PATH. Install it (e.g., brew install llama.cpp) or set LLAMA_SERVER_BIN to its absolute path")
	}
	return bin, nil
}

func (m *appModel) startServerCmd(selected modelItem, port string) tea.Cmd {
	return func() tea.Msg {
		// Do not mutate model state here; return it via a message and let Update handle it.
		// This avoids pointer-to-model mutations outside of the Update loop.

		ctx, cancel := context.WithCancel(context.Background())
		// Resolve llama-server binary
		bin, binErr := getLlamaServerBinary()
		if binErr != nil {
			cancel()
			return startErrorMsg{err: binErr}
		}
		cmd := exec.CommandContext(ctx, bin, "-m", selected.path, "--port", port, "--jinja")
		cmdEnv := os.Environ()
		cmd.Env = cmdEnv

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return startErrorMsg{err: fmt.Errorf("failed to create stdout pipe: %w", err)}
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			return startErrorMsg{err: fmt.Errorf("failed to create stderr pipe: %w", err)}
		}

		// Prepare file logging if enabled
		var fileWriter io.WriteCloser
		var logFilePath string
		if m.logToFileEnabled {
			_ = os.MkdirAll(m.logsDir, 0o755)
			filename := time.Now().Format("20060102_150405") + ".log"
			filePath := filepath.Join(m.logsDir, filename)
			f, ferr := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if ferr != nil {
				// If file cannot be opened, continue without file
			} else {
				logFilePath = filePath
				fileWriter = f
			}
		}

		logChan := make(chan string, 1024)
		exitChan := make(chan error, 1)

		// Start the command synchronously to catch immediate errors
		err = cmd.Start()
		if err != nil {
			cancel()
			return startErrorMsg{err: fmt.Errorf("failed to start llama-server: %w", err)}
		}

		// Emit quick diagnostics to the log channel for visibility
		select {
		case logChan <- fmt.Sprintf("Resolved llama-server binary: %s", bin):
		default:
		}
		select {
		case logChan <- fmt.Sprintf("Exec: %s -m %s --port %s --jinja", bin, selected.path, port):
		default:
		}
		select {
		case logChan <- "Waiting for server to become ready...":
		default:
		}

		// Reader goroutine - always streams logs to TUI regardless of file logging
		go func() {
			defer func() {
				if fileWriter != nil {
					_ = fileWriter.Close()
				}
			}()

			stdoutScanner := bufio.NewScanner(stdout)
			stderrScanner := bufio.NewScanner(stderr)
			stdoutScanner.Buffer(make([]byte, 1024), 1024*1024)
			stderrScanner.Buffer(make([]byte, 1024), 1024*1024)

			var wg sync.WaitGroup
			wg.Add(2)
			copyFn := func(scanner *bufio.Scanner) {
				defer wg.Done()
				for scanner.Scan() {
					line := scanner.Text()
					// Always write to file if enabled
					if fileWriter != nil {
						_, _ = io.WriteString(fileWriter, line+"\n")
					}
					// Always send to log channel for TUI display
					select {
					case logChan <- line:
					default:
						// In case UI is slow, drop oldest by non-blocking send
						// to prevent deadlocks; best-effort logging in UI.
					}
				}
			}
			go copyFn(stdoutScanner)
			go copyFn(stderrScanner)
			wg.Wait()
			// Close the log channel only after both stdout and stderr are fully read
			close(logChan)
		}()

		// Readiness probe goroutine - check when port starts accepting connections
		go func() {
			addresses := []string{"127.0.0.1:" + port, "[::1]:" + port}
			deadline := time.Now().Add(90 * time.Second)
			dialTimeout := 500 * time.Millisecond
			for {
				// Stop probing if process has exited (exitChan would close soon after)
				select {
				case <-exitChan:
					return
				default:
				}
				ready := false
				for _, addr := range addresses {
					conn, cerr := net.DialTimeout("tcp", addr, dialTimeout)
					if cerr == nil {
						_ = conn.Close()
						ready = true
						break
					}
				}
				if ready {
					select {
					case logChan <- fmt.Sprintf("Ready: listening on port %s", port):
					default:
					}
					return
				}
				if time.Now().After(deadline) {
					select {
					case logChan <- fmt.Sprintf("Warning: no readiness detected on port %s after 90s. It may still be loading the model (20B models can take a while).", port):
					default:
					}
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
		}()

		// Wait goroutine - monitors process exit
		go func() {
			waitErr := cmd.Wait()
			exitChan <- waitErr
			close(exitChan)
		}()

		// Return process state via message; Update will attach it to the model.
		return startedWithStateMsg{
			logChan:     logChan,
			exitChan:    exitChan,
			ctx:         ctx,
			cancel:      cancel,
			cmd:         cmd,
			modelName:   selected.name,
			port:        port,
			logFilePath: logFilePath,
		}
	}
}

func (m appModel) waitForLogLine() tea.Cmd {
	if m.logChan == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-m.logChan
		if !ok {
			return nil
		}
		return logLineMsg{text: line}
	}
}

func (m appModel) waitForExit() tea.Cmd {
	if m.exitChan == nil {
		return nil
	}
	return func() tea.Msg {
		err, ok := <-m.exitChan
		if !ok {
			return serverExitedMsg{err: nil}
		}
		return serverExitedMsg{err: err}
	}
}

func (m *appModel) stopServerCmd() tea.Cmd {
	return func() tea.Msg {
		if m.serverCmd == nil {
			return nil
		}
		// Attempt graceful stop - don't return stoppedMsg here
		// Wait for serverExitedMsg to confirm actual exit
		if m.serverCancel != nil {
			m.serverCancel()
		}
		if m.serverCmd.Process != nil {
			// Best-effort graceful signals
			_ = m.serverCmd.Process.Signal(os.Interrupt)
			_ = m.serverCmd.Process.Signal(syscall.SIGTERM)
			// Escalate to SIGKILL after a short grace period, without blocking UI
			go func(cmd *exec.Cmd) {
				timer := time.NewTimer(2 * time.Second)
				defer timer.Stop()
				<-timer.C
				if cmd != nil && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}(m.serverCmd)
		}
		return nil
	}
}
