package core

import (
	"os"
	"path/filepath"
	"testing"
)

// コピー元の走査は1回だけ行い、キャッシュを使い回すこと
// （進捗の分母と照合の集計で同じ木を何度も走査しない）。
func TestJobSrcScanCaches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := &Job{Src: dir}
	first := j.SrcScan()
	if first.Files != 1 || first.Bytes != 5 {
		t.Fatalf("SrcScan = %+v, want {Files:1 Bytes:5}", first)
	}
	// 走査後にファイルを足しても、キャッシュは変わらない（コピー元は処理中不変）
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if again := j.SrcScan(); again != first {
		t.Errorf("2回目はキャッシュを返すはず: %+v vs %+v", again, first)
	}
}

// SetSrcScan で走査済みの値を与えると、実際には走査せずその値を使うこと。
func TestJobSetSrcScan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := &Job{Src: dir}
	j.SetSrcScan(ScanResult{Files: 9, Bytes: 99})
	if got := j.SrcScan(); got.Files != 9 || got.Bytes != 99 {
		t.Errorf("SetSrcScan が反映されない: %+v", got)
	}
}
