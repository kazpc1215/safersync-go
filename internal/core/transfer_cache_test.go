package core

import (
	"os"
	"path/filepath"
	"testing"
)

// 転送の進捗の分母(Total)は「開始時の転送先(Base) + 今回の転送量(PlanBytes)」。
// 総ファイルサイズではないため、差分コピーでも開始直後に 100% にならない。
func TestTransferTotalIsBasePlusPlan(t *testing.T) {
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
	if want := tr.Base + 12345; tr.Total != want {
		t.Errorf("Total は Base+PlanBytes のはず: got %d, want %d", tr.Total, want)
	}
	_ = tr.Wait()
}
