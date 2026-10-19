package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 転送の進捗は転送先を走査せず、PlanBytes を分母に rsync の出力から数える。
func TestTransferTotalIsPlanBytes(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := &Job{Src: src, Dst: dst, FAT: true, ListFiles: true, PlanBytes: 12345}
	tr := j.StartTransfer()
	if err := tr.StartError(); err != nil {
		t.Fatal(err)
	}
	defer tr.Cleanup()
	if tr.Total != 12345 {
		t.Errorf("Total は PlanBytes のはず: got %d, want 12345", tr.Total)
	}
	if rc := tr.Wait(); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	_, done, total, _, _ := tr.Frame()
	if total != 12345 {
		t.Errorf("Frame total = %d, want 12345", total)
	}
	if done != 5 {
		t.Errorf("Frame done = %d, want 5（rsync の %%l から数える）", done)
	}
}

// 上書き更新でも転送先の再走査に頼らず 100% まで到達すること（回帰）。
func TestTransferFrameReaches100OnOverwrite(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 同じ長さの別内容（サイズだけ見ると差分なし）を置き、時刻だけ変えて転送させる。
	dstFile := filepath.Join(dst, "a.txt")
	if err := os.WriteFile(dstFile, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1000, 0)
	if err := os.Chtimes(dstFile, old, old); err != nil {
		t.Fatal(err)
	}

	j := &Job{Src: src, Dst: dst, FAT: true, ListFiles: true, PlanBytes: 5}
	tr := j.StartTransfer()
	if err := tr.StartError(); err != nil {
		t.Fatal(err)
	}
	defer tr.Cleanup()
	if rc := tr.Wait(); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	pct, done, total, _, _ := tr.Frame()
	if done != 5 || total != 5 || pct != 100 {
		t.Errorf("Frame = pct %d, done %d, total %d（want 100, 5, 5）", pct, done, total)
	}
}
