package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type tuiHistory struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

func historyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "safersync", "tui.json")
}

func loadHistory() (string, string) {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		return "", ""
	}
	var h tuiHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return "", ""
	}
	return h.Src, h.Dst
}

func saveHistory(src, dst string) {
	if src == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(historyPath()), 0o755)
	data, _ := json.Marshal(tuiHistory{Src: src, Dst: dst})
	_ = os.WriteFile(historyPath(), data, 0o644)
}
