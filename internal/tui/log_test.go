package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// ログ表示はファイル全体を毎レンダー読み直さず、開いたとき/キー操作のときに
// 読み直してキャッシュすること。
func TestLogLinesCached(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "safersync.log")
	if err := os.WriteFile(logPath, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := tbase()
	m.logFile = logPath
	m.showLog = true
	m.refreshLog()
	if len(m.logLines) != 1 || m.logLines[0] != "first" {
		t.Fatalf("logLines = %q", m.logLines)
	}

	// ファイルを書き換えても、refreshLog するまではキャッシュのまま
	if err := os.WriteFile(logPath, []byte("first\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(m.logLines) != 1 {
		t.Errorf("refreshLog 前なのにキャッシュが変わった: %q", m.logLines)
	}

	m.refreshLog()
	if len(m.logLines) != 2 {
		t.Errorf("refreshLog 後に反映されていない: %q", m.logLines)
	}

	// 空ログは案内の1行になる
	empty := filepath.Join(t.TempDir(), "empty.log")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.logFile = empty
	m.refreshLog()
	if len(m.logLines) != 1 || m.logLines[0] != "（ログはまだありません）" {
		t.Errorf("空ログの案内が出ない: %q", m.logLines)
	}
}
