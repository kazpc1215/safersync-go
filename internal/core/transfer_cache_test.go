package core

import (
	"os"
	"path/filepath"
	"testing"
)

// 転送の分母（Total）は SrcScan のキャッシュを使い、開始時にコピー元を
// 走査し直さないこと（TUI はプレビュー結果を引き継ぐ）。
func TestTransferUsesCachedSrcScan(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := &Job{Src: src, Dst: dst, FAT: true, ListFiles: true}
	j.SetSrcScan(ScanResult{Files: 3, Bytes: 12345})
	tr := j.StartTransfer()
	if err := tr.StartError(); err != nil {
		t.Fatal(err)
	}
	defer tr.Cleanup()
	if tr.Total != 12345 {
		t.Errorf("転送の Total は SrcScan のキャッシュを使うはず: %d", tr.Total)
	}
	_ = tr.Wait()
}
