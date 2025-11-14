package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// list item for models
type modelItem struct {
	name string
	path string
}

func (m modelItem) Title() string       { return m.name }
func (m modelItem) Description() string { return m.path }
func (m modelItem) FilterValue() string { return m.name }

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
