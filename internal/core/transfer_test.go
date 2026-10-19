package core

// transfer_test.go - TUI 転送（Transfer）の回帰テスト:
//   * rsync の出力から転送中ファイル名が取れる（-q で抑制されない）
//   * 出力用の一時ファイルが Cleanup で消える

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransferCurrentFileAndCleanup(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}

	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	j := &Job{Src: src, Dst: dst, FAT: true, ListFiles: true}
	tr := j.StartTransfer()
	if err := tr.StartError(); err != nil {
		t.Fatal(err)
	}
	logFile := tr.LogFile()
	if logFile == "" {
		t.Fatal("rsync 出力用の一時ファイルが作成されていない")
	}

	if rc := tr.Wait(); rc != 0 {
		t.Fatalf("rsync が失敗: rc=%d", rc)
	}
	if got := tr.CurrentFile(); !strings.Contains(got, "a.txt") {
		t.Errorf("転送したファイル名が取得できない: %q", got)
	}

	tr.Cleanup()
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Errorf("Cleanup 後も一時ファイルが残っている: %s", logFile)
	}
}

// Cleanup は Wait を呼ばれなかった転送でも一時ファイルを閉じる（Close 漏れ防止）。
func TestTransferCleanupClosesLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	tr := &Transfer{log: f, logFile: path}
	tr.Cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Cleanup 後もファイルが残っている: %v", err)
	}
	// 閉じられていること（同じ *os.File への Write はエラーになる）
	if _, err := f.WriteString("x"); err == nil {
		t.Error("Cleanup でファイルが閉じられていない")
	}
	tr.Cleanup() // 二重呼びでも安全
}
