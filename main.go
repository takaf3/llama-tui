package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	appTitle                    = "llama-tui"
	llamaBarnRelativeDir        = ".llamabarn"
	logsRelativeDir             = "llama-server-logs"
	defaultPort                 = "8080"
	logBufferSoftLimitCharacters = 2_000_000
)

// list item for models
type modelItem struct {
	name string
	path string
}

func (m modelItem) Title() string       { return m.name }
func (m modelItem) Description() string { return m.path }
func (m modelItem) FilterValue() string { return m.name }

// tea messages
type (
	scanDoneMsg struct {
		items []list.Item
		err   error
	}
	logLineMsg struct {
		text string
	}
	serverExitedMsg struct {
		err error
	}
	startedMsg struct{}
	startedWithStateMsg struct {
		logChan     chan string
		exitChan    chan error
		ctx         context.Context
		cancel      context.CancelFunc
		cmd         *exec.Cmd
		modelName   string
		port        string
		logFilePath string
	}
	startErrorMsg struct {
		err error
	}
	stoppedMsg struct {
		err error
	}
)

// model state
type appModel struct {
	width  int
	height int

	styles         uiStyles
	modelsList     list.Model
	portInput      textinput.Model
	logsViewport   viewport.Model
	statusLineText string

	homeDir           string
	barnDir           string
	logsDir           string
	logToFileEnabled  bool
	logFile           *os.File
	logFilePath       string
	logChan           chan string
	exitChan          chan error
	serverCmd         *exec.Cmd
	serverCtx         context.Context
	serverCancel      context.CancelFunc
	serverRunning    bool
	currentModelName string
	currentPort      string
	logBuffer        bytes.Buffer
	logBufferMu      sync.Mutex
}

type uiStyles struct {
	title        lipgloss.Style
	status       lipgloss.Style
	sectionTitle lipgloss.Style
	help         lipgloss.Style
	accent       lipgloss.Style
	border       lipgloss.Style
	statusRunning lipgloss.Style
	statusStopped lipgloss.Style
}

func newStyles() uiStyles {
	return uiStyles{
		title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		status:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		sectionTitle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141")),
		help:          lipgloss.NewStyle().Foreground(lipgloss.Color("246")),
		accent:        lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		statusRunning: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46")).Background(lipgloss.Color("22")).Padding(0, 1),
		statusStopped: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244")).Background(lipgloss.Color("235")).Padding(0, 1),
	}
}

func initialModel() appModel {
	styles := newStyles()

	home, _ := os.UserHomeDir()
	barnDir := filepath.Join(home, llamaBarnRelativeDir)
	logsDir := filepath.Join(barnDir, logsRelativeDir)

	items := []list.Item{}
	mdlList := list.New(items, list.NewDefaultDelegate(), 0, 0)
	mdlList.Title = "Models in " + barnDir
	mdlList.DisableQuitKeybindings()
	mdlList.SetShowHelp(false)
	mdlList.SetFilteringEnabled(true)

	port := textinput.New()
	port.Placeholder = "port"
	port.SetValue(defaultPort)
	port.CharLimit = 5
	port.Prompt = "Port: "

	vp := viewport.New(0, 0)
	vp.SetContent("")

	m := appModel{
		styles:           styles,
		modelsList:       mdlList,
		portInput:        port,
		logsViewport:     vp,
		statusLineText:   "Ready",
		homeDir:          home,
		barnDir:          barnDir,
		logsDir:          logsDir,
		logToFileEnabled: false,
		logChan:          nil,
		exitChan:         nil,
		serverCmd:        nil,
		serverRunning:    false,
		currentModelName: "",
		currentPort:      "",
	}

	return m
}

func (m appModel) Init() tea.Cmd {
	return m.scanModelsCmd()
}

func (m appModel) scanModelsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := scanModels(m.barnDir)
		return scanDoneMsg{items: items, err: err}
	}
}

func scanModels(barnDir string) ([]list.Item, error) {
	info, err := os.Stat(barnDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []list.Item{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", barnDir)
	}

	var items []list.Item
	err = filepath.WalkDir(barnDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".gguf") {
			rel, _ := filepath.Rel(barnDir, path)
			items = append(items, modelItem{
				name: rel,
				path: path,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

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
		return "", fmt.Errorf("llama-server not found in PATH. Install it (e.g., brew install llama.cpp) or set LLAMA_SERVER_BIN to its absolute path. PATH=%s", os.Getenv("PATH"))
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
						break;
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
			return stoppedMsg{}
		}
		// Attempt graceful stop
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
		return stoppedMsg{}
	}
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.resizeComponents(msg.Width, msg.Height)

	case tea.MouseMsg:
		// Route mouse wheel events to the logs viewport and do not update the models list with them.
		switch msg.Type {
		case tea.MouseWheelUp, tea.MouseWheelDown, tea.MouseWheelLeft, tea.MouseWheelRight:
			var cmd tea.Cmd
			m.logsViewport, cmd = m.logsViewport.Update(msg)
			return m, cmd
		default:
			// Ignore other mouse events for now
			return m, nil
		}

	case scanDoneMsg:
		if msg.err != nil {
			m.statusLineText = fmt.Sprintf("Scan error: %v", msg.err)
		} else {
			m.modelsList.SetItems(msg.items)
			m.statusLineText = fmt.Sprintf("Found %d model(s)", len(msg.items))
			if len(msg.items) > 0 && m.modelsList.Index() < 0 {
				m.modelsList.Select(0)
			}
		}
		return m, nil

	case startedMsg:
		// Start receiving logs and exit notifications
		return m, tea.Batch(m.waitForLogLine(), m.waitForExit())

	case startedWithStateMsg:
		// Attach process state to the model and begin receiving events
		m.serverCtx = msg.ctx
		m.serverCancel = msg.cancel
		m.serverCmd = msg.cmd
		m.logChan = msg.logChan
		m.exitChan = msg.exitChan
		m.serverRunning = true
		m.currentModelName = msg.modelName
		m.currentPort = msg.port
		m.logFilePath = msg.logFilePath
		m.statusLineText = fmt.Sprintf("Serving %s on port %s", msg.modelName, msg.port)
		return m, tea.Batch(m.waitForLogLine(), m.waitForExit())

	case startErrorMsg:
		// Handle start errors - don't mark as running
		m.statusLineText = fmt.Sprintf("Failed to start server: %v", msg.err)
		// Also surface error in logs panel so it's visible without scanning the status line
		m.logBufferMu.Lock()
		_, _ = m.logBuffer.WriteString("\nERROR: ")
		_, _ = m.logBuffer.WriteString(msg.err.Error())
		_, _ = m.logBuffer.WriteString("\n")
		m.logBufferMu.Unlock()
		m.logsViewport.SetContent(m.logBuffer.String())
		return m, nil

	case stoppedMsg:
		// Cleanup state
		m.serverRunning = false
		m.currentModelName = ""
		m.currentPort = ""
		m.serverCmd = nil
		m.serverCancel = nil
		m.logChan = nil
		m.exitChan = nil
		if m.logFile != nil {
			_ = m.logFile.Close()
			m.logFile = nil
		}
		m.logFilePath = ""
		m.statusLineText = "Server stopped"
		return m, nil

	case serverExitedMsg:
		m.serverRunning = false
		m.currentModelName = ""
		m.currentPort = ""
		m.serverCmd = nil
		m.serverCancel = nil
		m.logChan = nil
		m.exitChan = nil
		if m.logFile != nil {
			_ = m.logFile.Close()
			m.logFile = nil
		}
		m.logFilePath = ""
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.statusLineText = fmt.Sprintf("Server exited with error: %v", msg.err)
		} else {
			m.statusLineText = "Server exited"
		}
		return m, nil

	case logLineMsg:
		// Append to buffer (with trimming to soft limit)
		m.logBufferMu.Lock()
		_, _ = m.logBuffer.WriteString(msg.text)
		_, _ = m.logBuffer.WriteString("\n")
		if m.logBuffer.Len() > logBufferSoftLimitCharacters {
			// Trim oldest half to keep memory bounded
			data := m.logBuffer.Bytes()
			start := len(data) / 2
			var newBuf bytes.Buffer
			_, _ = newBuf.Write(data[start:])
			m.logBuffer = newBuf
		}
		m.logBufferMu.Unlock()

		m.logsViewport.SetContent(m.logBuffer.String())
		m.logsViewport.GotoBottom()
		if m.serverRunning {
			return m, m.waitForLogLine()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Ensure server is stopped
			if m.serverRunning {
				return m, tea.Sequence(m.stopServerCmd(), tea.Quit)
			}
			return m, tea.Quit
		case "r":
			return m, m.scanModelsCmd()
		case "l":
			// Toggle file logging (applies on next start)
			m.logToFileEnabled = !m.logToFileEnabled
			if m.logToFileEnabled {
				m.statusLineText = "Log to file: enabled (applies on next start)"
			} else {
				m.statusLineText = "Log to file: disabled"
			}
			return m, nil
		case "p":
			if m.portInput.Focused() {
				m.portInput.Blur()
			} else {
				m.portInput.Focus()
			}
			return m, nil
		case "s":
			if m.serverCmd != nil {
				// Provide immediate user feedback
				m.statusLineText = "Stopping server..."
				m.logBufferMu.Lock()
				_, _ = m.logBuffer.WriteString("\n[ui] Stopping server...\n")
				m.logBufferMu.Unlock()
				m.logsViewport.SetContent(m.logBuffer.String())
				return m, m.stopServerCmd()
			}
			return m, nil
		case "enter":
			// Start server on selected model
			if m.serverRunning {
				return m, nil
			}
			item, ok := m.modelsList.SelectedItem().(modelItem)
			if !ok {
				m.statusLineText = "No model selected"
				return m, nil
			}
			portStr := strings.TrimSpace(m.portInput.Value())
			if portStr == "" {
				portStr = defaultPort
			}
			// Validate port before starting server
			portNum, err := validatePort(portStr)
			if err != nil {
				m.statusLineText = fmt.Sprintf("Invalid port: %v", err)
				return m, nil
			}
			portStr = strconv.Itoa(portNum)
			// Clear logs for a new session and set initial message
			m.logBufferMu.Lock()
			m.logBuffer.Reset()
			initialMsg := fmt.Sprintf("Starting llama-server with model: %s on port: %s...", item.name, portStr)
			_, _ = m.logBuffer.WriteString(initialMsg)
			m.logBufferMu.Unlock()
			m.logsViewport.SetContent(initialMsg)
			m.statusLineText = fmt.Sprintf("Starting %s on port %s...", item.name, portStr)
			return m, m.startServerCmd(item, portStr)
		}
		// Update nested components for unhandled keys
		var cmd tea.Cmd
		m.modelsList, cmd = m.modelsList.Update(msg)
		var portCmd tea.Cmd
		m.portInput, portCmd = m.portInput.Update(msg)
		return m, tea.Batch(cmd, portCmd)
	}

	// Default: update nested components
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.modelsList, cmd = m.modelsList.Update(msg)
	cmds = append(cmds, cmd)
	m.portInput, cmd = m.portInput.Update(msg)
	cmds = append(cmds, cmd)
	m.logsViewport, cmd = m.logsViewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m appModel) resizeComponents(width, height int) (tea.Model, tea.Cmd) {
	if width <= 0 || height <= 0 {
		return m, nil
	}

	headerHeight := 1
	footerHeight := 2
	contentHeight := height - headerHeight - footerHeight - 2
	if contentHeight < 5 {
		contentHeight = 5
	}
	leftWidth := width / 3
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := width - leftWidth - 4
	if rightWidth < 20 {
		rightWidth = 20
	}

	m.modelsList.SetSize(leftWidth, contentHeight)
	m.logsViewport.Width = rightWidth
	m.logsViewport.Height = contentHeight
	return m, nil
}

func (m appModel) View() string {
	// Render status chip
	var statusChip string
	if m.serverRunning {
		statusChip = m.styles.statusRunning.Render("[RUNNING]")
	} else {
		statusChip = m.styles.statusStopped.Render("[STOPPED]")
	}
	
	// Build header with status chip and model info
	headerParts := []string{
		m.styles.title.Render(appTitle),
		statusChip,
	}
	if m.serverRunning && m.currentModelName != "" && m.currentPort != "" {
		headerParts = append(headerParts, m.styles.accent.Render(fmt.Sprintf("%s:%s", m.currentModelName, m.currentPort)))
	}
	headerParts = append(headerParts, m.styles.status.Render(m.statusLineText))
	header := strings.Join(headerParts, "  ")

	left := m.styles.border.Render(m.styles.sectionTitle.Render("Models") + "\n" + m.modelsList.View())
	logTitle := "Logs"
	if m.logToFileEnabled {
		logTitle += " (file: on)"
	} else {
		logTitle += " (file: off)"
	}
	if m.logFilePath != "" && m.serverRunning {
		logTitle += " -> " + filepath.Base(m.logFilePath)
	}
	right := m.styles.border.Render(m.styles.sectionTitle.Render(logTitle) + "\n" + m.logsViewport.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	helpLines := []string{
		m.styles.help.Render("[enter] start  [s] stop  [r] refresh  [p] toggle port input  [l] toggle file log  [q] quit"),
		m.styles.help.Render("Port: ") + m.portInput.View(),
	}
	footer := strings.Join(helpLines, "\n")

	return header + "\n\n" + content + "\n\n" + footer
}

func main() {
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}


