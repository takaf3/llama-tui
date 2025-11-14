package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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

	m.leftWidth = leftWidth
	m.rightWidth = rightWidth
	m.contentHeight = contentHeight

	m.modelsList.SetSize(leftWidth, contentHeight)
	m.logsViewport.Width = rightWidth
	m.logsViewport.Height = contentHeight
	return m, nil
}

func (m appModel) colorLog(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error"):
		return m.styles.logError.Render(line)
	case strings.Contains(lower, "warn"):
		return m.styles.logWarn.Render(line)
	case strings.Contains(lower, "info"):
		return m.styles.logInfo.Render(line)
	default:
		return line
	}
}

func (m appModel) renderPanelWithTitle(title, body string, contentWidth int) string {
	borderStyle := m.styles.panelBorder
	titleStyled := m.styles.panelTitle.Render(" " + title + " ")

	// Total width includes border characters (2 chars for left/right borders)
	total := contentWidth + 2

	// Top line with title embedded
	topLeft := borderStyle.Render("╭")
	topRight := borderStyle.Render("╮")
	horiz := borderStyle.Render("─")
	titleW := lipgloss.Width(titleStyled)
	padLeft := 1
	padRight := total - 2 - titleW - padLeft
	if padRight < 0 {
		padRight = 0
	}
	top := topLeft + strings.Repeat(horiz, padLeft) + titleStyled + strings.Repeat(horiz, padRight) + topRight

	// Body lines framed with borders
	left := borderStyle.Render("│")
	right := borderStyle.Render("│")
	var b strings.Builder
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w < contentWidth {
			line += strings.Repeat(" ", contentWidth-w)
		}
		b.WriteString(left)
		b.WriteString(line)
		b.WriteString(right)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}

	// Bottom
	bottom := borderStyle.Render("╰" + strings.Repeat("─", total-2) + "╯")

	return top + "\n" + b.String() + "\n" + bottom
}

func (m appModel) View() string {
	// Render status chip
	var statusChip string
	if m.serverStopping {
		statusChip = m.styles.statusStopping.Render("[STOPPING]")
	} else if m.serverRunning {
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
	// Use warning style for confirmation messages, regular status style otherwise
	if m.confirmAction != confirmNone {
		headerParts = append(headerParts, m.styles.confirmWarning.Render(m.statusLineText))
	} else {
		headerParts = append(headerParts, m.styles.status.Render(m.statusLineText))
	}
	header := strings.Join(headerParts, "  ")

	left := m.renderPanelWithTitle("Models", m.modelsList.View(), m.leftWidth)
	logTitle := "Logs"
	if m.logToFileEnabled {
		logTitle += " (file: on)"
	} else {
		logTitle += " (file: off)"
	}
	if m.logFilePath != "" && m.serverRunning {
		logTitle += " -> " + filepath.Base(m.logFilePath)
	}
	right := m.renderPanelWithTitle(logTitle, m.logsViewport.View(), m.rightWidth)

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	// Build explicit status bar
	var statusText string
	if m.serverStopping {
		statusText = "Status: " + m.styles.statusStopping.Render("[STOPPING]")
	} else if m.serverRunning {
		statusText = "Status: " + m.styles.statusRunning.Render("[RUNNING]")
	} else {
		statusText = "Status: " + m.styles.statusStopped.Render("[STOPPED]")
	}

	if m.currentModelName != "" {
		statusText += " • Model: " + m.styles.accent.Render(m.currentModelName)
	}
	if m.currentPort != "" {
		statusText += " • Port: " + m.styles.accent.Render(m.currentPort)
	}
	statusBar := m.styles.status.Render(statusText)

	// State-based help line
	var helpLine string
	if m.confirmAction == confirmQuit {
		helpLine = m.styles.confirmWarning.Render("Quit? Press q again to confirm, esc to cancel")
	} else if m.confirmAction == confirmStop {
		helpLine = m.styles.confirmWarning.Render("Stop server? Press s again to confirm, esc to cancel")
	} else if m.serverStopping {
		helpLine = m.styles.help.Render("Stopping server... Please wait")
	} else if m.serverRunning {
		helpLine = m.styles.help.Render("[s] stop  [h] help  [q] quit")
	} else {
		helpLine = m.styles.help.Render("[enter] start  [r] refresh  [p] toggle port  [l] toggle file log  [h] help  [q] quit")
	}

	// Render port input - dimmed if server is running/stopping
	portInputView := m.portInput.View()
	if m.serverRunning || m.serverStopping {
		portInputView = m.styles.disabled.Render(portInputView)
	}

	helpLines := []string{
		statusBar,
		helpLine,
		m.styles.help.Render("Port: ") + portInputView,
	}
	footer := strings.Join(helpLines, "\n")

	view := header + "\n\n" + content + "\n\n" + footer

	// Show help overlay if enabled
	if m.showHelp {
		helpContent := []string{
			"Keyboard Shortcuts:",
			"",
			"  [enter]  Start server with selected model",
			"  [s]      Stop the running server (press twice to confirm)",
			"  [r]      Refresh/rescan models list",
			"  [p]      Focus/unfocus port input",
			"  [l]      Toggle file logging (applies on next start)",
			"  [h]      Toggle this help overlay",
			"  [esc]    Cancel confirmation, close help, or unfocus port",
			"  [q]      Quit (press twice to confirm; stops server if running)",
			"  [ctrl+c] Quit immediately (bypasses confirmation)",
			"",
			"Status Indicators:",
			"  [RUNNING]  Server is active",
			"  [STOPPING] Server shutdown in progress",
			"  [STOPPED]  No server running",
			"",
			"Press [h] or [esc] to close this help",
		}
		helpText := strings.Join(helpContent, "\n")
		helpWidth := m.width - 8
		if helpWidth < 50 {
			helpWidth = 50
		}
		helpPanel := m.renderPanelWithTitle("Help", helpText, helpWidth)
		// Overlay help centered on top of the view
		overlay := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpPanel)
		return overlay
	}

	return view
}
