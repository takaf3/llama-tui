package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

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
	startedMsg          struct{}
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

// confirmation action type
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmStop
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

	leftWidth     int
	rightWidth    int
	contentHeight int

	homeDir          string
	barnDir          string
	logsDir          string
	logToFileEnabled bool
	logFile          *os.File
	logFilePath      string
	logChan          chan string
	exitChan         chan error
	serverCmd        *exec.Cmd
	serverCtx        context.Context
	serverCancel     context.CancelFunc
	serverRunning    bool
	serverStopping   bool
	pendingQuit      bool
	showHelp         bool
	currentModelName string
	currentPort      string
	logBuffer        bytes.Buffer
	confirmAction    confirmAction
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
		serverStopping:   false,
		pendingQuit:      false,
		showHelp:         false,
		currentModelName: "",
		currentPort:      "",
		confirmAction:    confirmNone,
	}

	return m
}

func (m appModel) Init() tea.Cmd {
	return m.scanModelsCmd()
}
