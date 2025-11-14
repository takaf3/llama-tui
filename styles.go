package main

import "github.com/charmbracelet/lipgloss"

type uiStyles struct {
	title          lipgloss.Style
	status         lipgloss.Style
	sectionTitle   lipgloss.Style
	help           lipgloss.Style
	accent         lipgloss.Style
	border         lipgloss.Style
	statusRunning  lipgloss.Style
	statusStopping lipgloss.Style
	statusStopped  lipgloss.Style
	panelBorder    lipgloss.Style
	panelTitle     lipgloss.Style
	logError       lipgloss.Style
	logWarn        lipgloss.Style
	logInfo        lipgloss.Style
	disabled       lipgloss.Style
}

func newStyles() uiStyles {
	// Catppuccin Mocha color palette
	return uiStyles{
		title:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b4befe")), // lavender
		status:         lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8")),            // subtext0
		sectionTitle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b4befe")), // lavender
		help:           lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8")),            // subtext0
		accent:         lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")),            // blue
		border:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		statusRunning:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#a6e3a1")).Background(lipgloss.Color("#313244")).Padding(0, 1), // green on surface0
		statusStopping: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f9e2af")).Background(lipgloss.Color("#313244")).Padding(0, 1), // yellow on surface0
		statusStopped:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6c7086")).Background(lipgloss.Color("#313244")).Padding(0, 1), // overlay1 on surface0
		panelBorder:    lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")),                                                                // overlay1
		panelTitle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#b4befe")),                                                     // lavender
		logError:       lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")),                                                                // red
		logWarn:        lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")),                                                                // yellow
		logInfo:        lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")),                                                                // blue
		disabled:       lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")),                                                                // overlay1 (dimmed)
	}
}
