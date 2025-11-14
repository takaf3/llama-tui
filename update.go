package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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
		m.serverStopping = false
		m.currentModelName = msg.modelName
		m.currentPort = msg.port
		m.logFilePath = msg.logFilePath
		m.statusLineText = fmt.Sprintf("Serving %s on port %s", msg.modelName, msg.port)
		// Blur port input when server starts
		if m.portInput.Focused() {
			m.portInput.Blur()
		}
		return m, tea.Batch(m.waitForLogLine(), m.waitForExit())

	case startErrorMsg:
		// Handle start errors - don't mark as running
		m.statusLineText = fmt.Sprintf("Failed to start server: %v", msg.err)
		// Also surface error in logs panel so it's visible without scanning the status line
		errorMsg := "\nERROR: " + msg.err.Error() + "\n"
		coloredError := m.colorLog(errorMsg)
		_, _ = m.logBuffer.WriteString(coloredError)
		m.logsViewport.SetContent(m.logBuffer.String())
		return m, nil

	case stoppedMsg:
		// This message is no longer used - cleanup happens in serverExitedMsg
		return m, nil

	case serverExitedMsg:
		// Cleanup state - this is where we actually confirm the server has stopped
		m.serverRunning = false
		m.serverStopping = false
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
			m.statusLineText = fmt.Sprintf("Server stopped (error: %v)", msg.err)
			stopMsg := fmt.Sprintf("\n[ui] Server stopped with error: %v\n", msg.err)
			coloredStopMsg := m.colorLog(stopMsg)
			_, _ = m.logBuffer.WriteString(coloredStopMsg)
			m.logsViewport.SetContent(m.logBuffer.String())
		} else {
			m.statusLineText = "Server stopped"
			stopMsg := "\n[ui] Server stopped successfully\n"
			coloredStopMsg := m.colorLog(stopMsg)
			_, _ = m.logBuffer.WriteString(coloredStopMsg)
			m.logsViewport.SetContent(m.logBuffer.String())
		}
		// If quit was pending, now quit
		if m.pendingQuit {
			return m, tea.Quit
		}
		return m, nil

	case logLineMsg:
		// Append to buffer (with trimming to soft limit)
		coloredLine := m.colorLog(msg.text)
		_, _ = m.logBuffer.WriteString(coloredLine)
		_, _ = m.logBuffer.WriteString("\n")
		if m.logBuffer.Len() > logBufferSoftLimitCharacters {
			// Trim oldest half to keep memory bounded
			data := m.logBuffer.Bytes()
			start := len(data) / 2
			var newBuf bytes.Buffer
			_, _ = newBuf.Write(data[start:])
			m.logBuffer = newBuf
		}

		m.logsViewport.SetContent(m.logBuffer.String())
		m.logsViewport.GotoBottom()
		if m.serverRunning {
			return m, m.waitForLogLine()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Ensure server is stopped before quitting
			if m.serverRunning && !m.serverStopping {
				m.pendingQuit = true
				m.serverStopping = true
				m.statusLineText = "Stopping server before quit..."
				stopMsg := "\n[ui] Stopping server before quit...\n"
				coloredStopMsg := m.colorLog(stopMsg)
				_, _ = m.logBuffer.WriteString(coloredStopMsg)
				m.logsViewport.SetContent(m.logBuffer.String())
				return m, m.stopServerCmd()
			}
			// If already stopping, just quit (will happen after serverExitedMsg)
			if m.serverStopping {
				return m, nil
			}
			return m, tea.Quit
		case "r":
			if m.serverRunning || m.serverStopping {
				m.statusLineText = "Cannot refresh while server is running"
				return m, nil
			}
			m.statusLineText = "Scanning for models..."
			return m, m.scanModelsCmd()
		case "l":
			if m.serverRunning || m.serverStopping {
				m.statusLineText = "Cannot toggle logging while server is running"
				return m, nil
			}
			// Toggle file logging (applies on next start)
			m.logToFileEnabled = !m.logToFileEnabled
			if m.logToFileEnabled {
				m.statusLineText = "Log to file: enabled (applies on next start)"
			} else {
				m.statusLineText = "Log to file: disabled"
			}
			return m, nil
		case "p":
			if m.serverRunning || m.serverStopping {
				m.statusLineText = "Cannot edit port while server is running"
				return m, nil
			}
			if m.portInput.Focused() {
				m.portInput.Blur()
				m.statusLineText = "Port input unfocused"
			} else {
				m.portInput.Focus()
				m.statusLineText = "Port input focused - type port number"
			}
			return m, nil
		case "s":
			if m.serverRunning && !m.serverStopping {
				m.serverStopping = true
				m.statusLineText = "Stopping server..."
				stopMsg := "\n[ui] Stopping server...\n"
				coloredStopMsg := m.colorLog(stopMsg)
				_, _ = m.logBuffer.WriteString(coloredStopMsg)
				m.logsViewport.SetContent(m.logBuffer.String())
				return m, m.stopServerCmd()
			}
			if m.serverStopping {
				m.statusLineText = "Server is already stopping..."
				return m, nil
			}
			if !m.serverRunning {
				m.statusLineText = "No server is running"
				return m, nil
			}
			return m, nil
		case "h":
			m.showHelp = !m.showHelp
			return m, nil
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			// If port input is focused, blur it on esc
			if m.portInput.Focused() {
				m.portInput.Blur()
				return m, nil
			}
			return m, nil
		case "enter":
			// Start server on selected model
			if m.serverRunning || m.serverStopping {
				m.statusLineText = "Server is already running or stopping"
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
			// Blur port input before starting server
			if m.portInput.Focused() {
				m.portInput.Blur()
			}
			// Clear logs for a new session and set initial message
			m.logBuffer.Reset()
			initialMsg := fmt.Sprintf("Starting llama-server with model: %s on port: %s...", item.name, portStr)
			coloredMsg := m.colorLog(initialMsg)
			_, _ = m.logBuffer.WriteString(coloredMsg)
			m.logsViewport.SetContent(coloredMsg)
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
