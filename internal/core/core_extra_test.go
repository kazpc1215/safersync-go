package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

// ログの場所は bash 版 env.sh と同じ優先順位で決めること:
// SAFERSYNC_LOG_DIR > XDG_STATE_HOME > (macOS: ~/Library/Logs) > XDG_DATA_HOME > ~/.local/share
func TestLogDirPrecedence(t *testing.T) {
	t.Setenv("SAFERSYNC_LOG_DIR", "/tmp/ss-log")
	t.Setenv("XDG_STATE_HOME", "/tmp/ss-state")
	t.Setenv("XDG_DATA_HOME", "/tmp/ss-data")
	t.Setenv("HOME", "/tmp/ss-home")

	if got := LogDir(); got != "/tmp/ss-log" {
		t.Errorf("SAFERSYNC_LOG_DIR が最優先のはず: %q", got)
	}
	t.Setenv("SAFERSYNC_LOG_DIR", "")
	if got := LogDir(); got != "/tmp/ss-state/safersync" {
		t.Errorf("XDG_STATE_HOME が2番目のはず: %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	if runtime.GOOS == "darwin" {
		if got := LogDir(); got != "/tmp/ss-home/Library/Logs/safersync" {
			t.Errorf("macOS は ~/Library/Logs が既定のはず: %q", got)
		}
		return
	}
	if got := LogDir(); got != "/tmp/ss-data/safersync" {
		t.Errorf("XDG_DATA_HOME を使うはず: %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "")
	if got := LogDir(); got != "/tmp/ss-home/.local/share/safersync" {
		t.Errorf("既定は ~/.local/share のはず: %q", got)
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

func TestLogMsgRotation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SAFERSYNC_LOG_DIR", dir)

	path := LogFile()
	big := make([]byte, logMaxBytes+1)
	if err := os.WriteFile(path, big, 0o644); err != nil {
		t.Fatal(err)
	}
	LogMsg("hello")
	if fi, err := os.Stat(path + ".1"); err != nil || fi.Size() != int64(len(big)) {
		t.Fatalf("5MiB 超でローテーションされない: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("ローテーション後の新ログに追記されていない: %q", data)
	}

	// 上限以下ならローテーションせず追記するだけ。
	LogMsg("world")
	if fi, _ := os.Stat(path + ".1"); fi.Size() != int64(len(big)) {
		t.Errorf("上限以下で退避ファイルが変わった: %d", fi.Size())
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "world") {
		t.Errorf("追記されていない: %q", data)
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
