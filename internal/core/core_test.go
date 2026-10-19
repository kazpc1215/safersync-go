package core

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestScanDirExcludes(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, 10), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("photo/a.jpg")           // 対象
	mk("photo/b.jpg")           // 対象
	mk(".DS_Store")             // 除外
	mk("._c.jpg")               // 除外（._*）
	mk(".Trashes/x")            // 除外（ディレクトリごと）
	mk("sub/.Spotlight-V100/y") // 除外（サブフォルダ内も）

	res := ScanDir(dir, DefaultExcludes)
	if res.Files != 2 || res.Bytes != 20 {
		t.Errorf("want 2 files / 20 bytes, got %d files / %d bytes", res.Files, res.Bytes)
	}

	// --exclude 'sub' で枝ごと除外される
	res2 := ScanDir(dir, append(append([]string{}, DefaultExcludes...), "sub", "photo"))
	if res2.Files != 0 {
		t.Errorf("want 0 files, got %d", res2.Files)
	}
}

// シンボリックリンクや FIFO はファイル数・バイト数に含めない（bash 版 find -type f と同じ）
func TestScanDirRegularFilesOnly(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(real, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "link.txt")); err != nil {
		t.Fatalf("シンボリックリンクを作成できない: %v", err)
	}
	if err := os.Symlink(dir, filepath.Join(dir, "linkdir")); err != nil {
		t.Fatalf("ディレクトリへのシンボリックリンクを作成できない: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644); err != nil {
		t.Logf("FIFO を作成できないためスキップ: %v", err)
	}

	res := ScanDir(dir, nil)
	if res.Files != 1 {
		t.Errorf("通常ファイル 1 件だけ数えるはず: %d", res.Files)
	}
	if res.Bytes != 5 {
		t.Errorf("Bytes は通常ファイルのサイズだけのはず: %d", res.Bytes)
	}
}

func TestFreeBytes(t *testing.T) {
	if got := FreeBytes(t.TempDir()); got <= 0 {
		t.Errorf("空き容量が取得できない: %d", got)
	}
}

func TestFsmtDurationAndBytes(t *testing.T) {
	if FmtDuration(3661) != "1時間1分" {
		t.Errorf("bad: %q", FmtDuration(3661))
	}
	if FmtBytes(512) != "512.0 B" {
		t.Errorf("bad: %q", FmtBytes(512))
	}
}
