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

// historyPath は前回使ったパスの保存先。XDG_STATE_HOME があればそれに従う。
func historyPath() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "safersync", "tui.json")
	}
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

// fillFromHistory は指定されていない方だけを履歴で補う。
// 明示指定（例: --to X）を履歴で上書きしないためのもの。
func fillFromHistory(from, to string) (string, string) {
	if from != "" && to != "" {
		return from, to
	}
	hsrc, hdst := loadHistory()
	if from == "" {
		from = hsrc
	}
	if to == "" {
		to = hdst
	}
	return from, to
}

func saveHistory(src, dst string) {
	if src == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(historyPath()), 0o700)
	data, _ := json.Marshal(tuiHistory{Src: src, Dst: dst})
	path := historyPath()
	_ = os.WriteFile(path, data, 0o600)
	_ = os.Chmod(path, 0o600)
}
