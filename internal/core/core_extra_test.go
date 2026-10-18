package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobAllPatterns(t *testing.T) {
	j := &Job{Excludes: []string{"*.LRV", "*.THM"}}
	pats := j.AllExcludePatterns()
	if len(pats) < len(DefaultExcludes)+2 {
		t.Fatalf("exclude patterns が不足: %v", pats)
	}
}

// スキャン中の割り込み: Ctx がキャンセルされていたら dry-run を実行せず、
// context のエラー（context.Canceled）として返すこと。
func TestCollectDiffContextCancel(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	j := &Job{Src: t.TempDir(), Dst: t.TempDir(), FAT: true, Ctx: ctx}
	_, err := j.CollectDiff("%i %n")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("キャンセル時のエラーが context.Canceled でない: %v", err)
	}
	// PlanChanges / VerifyOne もエラーとして扱い、成功扱いにならないこと
	if plan := j.PlanChanges(); plan.Err == nil {
		t.Error("キャンセル時に PlanChanges がエラーを返さない（差分なしと誤判定する）")
	}
}

func TestRsyncArgsListFiles(t *testing.T) {
	j := &Job{Src: "/tmp/src", Dst: "/tmp/dst", FAT: true, ListFiles: true}
	args := j.rsyncArgs()
	if !contains(args, "--out-format=%n") {
		t.Errorf("--out-format が付いていない: %v", args)
	}
	// -q は --out-format の出力（TUI のファイル名表示）を抑制してしまうため、併用しないこと
	if contains(args, "-q") {
		t.Errorf("ListFiles 時に -q が付いている（ファイル名が出ない）: %v", args)
	}
	j2 := &Job{Src: "/tmp/src", Dst: "/tmp/dst", FAT: true, ListFiles: false}
	args2 := j2.rsyncArgs()
	if contains(args2, "--out-format=%n") {
		t.Errorf("ListFiles=false なのに --out-format が付いている: %v", args2)
	}
	if !contains(args2, "-q") {
		t.Errorf("通常実行で -q が付いていない: %v", args2)
	}
}

func TestPlanChangesBasic(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := &Job{Src: src, Dst: dst, FAT: true}
	plan := j.PlanChanges()
	if plan.Files != 1 {
		t.Errorf("want 1 file, got %d", plan.Files)
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v || strings.Contains(s, v) {
			return true
		}
	}
	return false
}
