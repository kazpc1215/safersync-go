package core

import (
	"os"
	"path/filepath"
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

	res := ScanDir(dir, nil)
	if res.Files != 2 || res.Bytes != 20 {
		t.Errorf("want 2 files / 20 bytes, got %d files / %d bytes", res.Files, res.Bytes)
	}

	// --exclude 'sub' で枝ごと除外される
	res2 := ScanDir(dir, []string{"sub", "photo"})
	if res2.Files != 0 {
		t.Errorf("want 0 files, got %d", res2.Files)
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
