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
