package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

	// Regex to match multipart GGUF files case-insensitively: e.g., "model-00001-of-00003.gguf"
	multipartPattern := regexp.MustCompile(`(?i)^(.+)-(\d+)-of-(\d+)\.gguf$`)

	type groupedModel struct {
		item       modelItem
		shardIndex int
	}
	modelMap := make(map[string]groupedModel)

	err = filepath.WalkDir(barnDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".gguf") {
			return nil
		}

		rel, _ := filepath.Rel(barnDir, path)
		fileName := d.Name()

		// Check if this is a multipart file
		matches := multipartPattern.FindStringSubmatch(fileName)
		if matches != nil {
			dir := filepath.Dir(rel)
			var groupKey string
			if dir == "." {
				groupKey = strings.ToLower(matches[1])
			} else {
				groupKey = strings.ToLower(filepath.Join(dir, matches[1]))
			}

			shardNum, err := strconv.Atoi(matches[2])
			if err != nil {
				shardNum = 0
			}

			existing, exists := modelMap[groupKey]
			if !exists || shardNum < existing.shardIndex {
				var displayName string
				if dir == "." {
					displayName = matches[1] + ".gguf"
				} else {
					displayName = filepath.Join(dir, matches[1]+".gguf")
				}
				modelMap[groupKey] = groupedModel{
					item: modelItem{
						name: displayName,
						path: path,
					},
					shardIndex: shardNum,
				}
			}
		} else {
			modelMap[rel] = groupedModel{
				item: modelItem{
					name: rel,
					path: path,
				},
				shardIndex: 0,
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Convert map values to slice and sort by name
	items := make([]list.Item, 0, len(modelMap))
	for _, grouped := range modelMap {
		items = append(items, grouped.item)
	}

	// Sort by name for stable, predictable ordering
	sort.Slice(items, func(i, j int) bool {
		return items[i].(modelItem).name < items[j].(modelItem).name
	})

	return items, nil
}
