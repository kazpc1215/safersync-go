package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	if !contains(args, "--out-format=%i %l %n") {
		t.Errorf("--out-format が付いていない: %v", args)
	}
	// -q は --out-format の出力（TUI のファイル名表示）を抑制してしまうため、併用しないこと
	if contains(args, "-q") {
		t.Errorf("ListFiles 時に -q が付いている（ファイル名が出ない）: %v", args)
	}
	j2 := &Job{Src: "/tmp/src", Dst: "/tmp/dst", FAT: true, ListFiles: false}
	args2 := j2.rsyncArgs()
	if contains(args2, "--out-format") {
		t.Errorf("ListFiles=false なのに --out-format が付いている: %v", args2)
	}
	if !contains(args2, "-q") {
		t.Errorf("通常実行で -q が付いていない: %v", args2)
	}
}

// IsRemote の判定は rsync と同じ規則（最初の "/" より前の ":"）であること。
func TestIsRemote_edgeCases(t *testing.T) {
	remote := []string{
		"host:/abs/path",
		"host::module", // デーモン構文
		"rsync://user@host/module/sub",
		":hostless", // ホスト名が空でも rsync はリモート扱い
	}
	for _, p := range remote {
		if !IsRemote(p) {
			t.Errorf("IsRemote(%q) = false, want true", p)
		}
	}
	local := []string{
		"",
		"/abs/with:colon/path", // 最初の "/" より後の ":" はローカル
		"./weird:name",         // "./" でエスケープ（rsync と同じ回避策）
		"./2019-05-01_12:30:45",
		"~/dir",
		"dir/sub:file",
		"rsync_local", // "rsync://" ではない
	}
	for _, p := range local {
		if IsRemote(p) {
			t.Errorf("IsRemote(%q) = true, want false", p)
		}
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

// rsync の rc=24（転送中にファイルが消えた）は成功扱いにそろえる。
func TestIsVanishedExit(t *testing.T) {
	if !IsVanishedExit(24) {
		t.Error("rc=24 を vanished と判定していない")
	}
	for _, rc := range []int{0, 1, 23, 25, 130} {
		if IsVanishedExit(rc) {
			t.Errorf("rc=%d を vanished と誤判定した", rc)
		}
	}
}

// 巨大なツリーでも詳細リストは上限で打ち切り、件数は正しく数える。
func TestPlanChangesTruncatesDetails(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	for i := 0; i < maxPlanDetails+3; i++ {
		p := filepath.Join(src, fmt.Sprintf("f%05d", i))
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan := (&Job{Src: src, Dst: dst, FAT: true}).PlanChanges()
	if plan.Changes != maxPlanDetails+3 {
		t.Fatalf("Changes = %d, want %d", plan.Changes, maxPlanDetails+3)
	}
	if !plan.Truncated {
		t.Error("Truncated が立っていない")
	}
	if len(plan.Lines) != maxPlanDetails || len(plan.Transfer) != maxPlanDetails {
		t.Errorf("詳細の件数 = %d / %d, want %d", len(plan.Lines), len(plan.Transfer), maxPlanDetails)
	}
}

// 進捗用の --out-format 出力から差分行だけを取り除き、--stats の集計行は残す。
func TestWriteNonDiffOutput(t *testing.T) {
	data := []byte(strings.Join([]string{
		"cd+++++++++ 0 ./",
		">f+++++++++ 5 a.txt",
		".f...p..... 3 b.txt",
		"Number of files: 2 (reg: 2)",
		"Total file size: 8 bytes",
	}, "\n"))
	var out bytes.Buffer
	writeNonDiffOutput(&out, data)
	got := out.String()
	for _, item := range []string{">f+++++++++ 5 a.txt", ".f...p..... 3 b.txt", "cd+++++++++ 0 ./"} {
		if strings.Contains(got, item) {
			t.Errorf("差分行が出力に残っている: %q", item)
		}
	}
	for _, want := range []string{"Number of files: 2 (reg: 2)", "Total file size: 8 bytes"} {
		if !strings.Contains(got, want) {
			t.Errorf("統計行が出力されていない: %q\n%s", want, got)
		}
	}
}

func TestIsRsyncItemLine(t *testing.T) {
	for _, line := range []string{">f+++++++++ 5 a.txt", "cd+++++++++ 0 ./", ".d..t...... 1 x", "cL+++++++++ 6 link"} {
		if !isRsyncItemLine(line) {
			t.Errorf("差分として判定できない: %q", line)
		}
	}
	for _, line := range []string{"Number of files: 2", "Total file size: 8 bytes", "", ">f"} {
		if isRsyncItemLine(line) {
			t.Errorf("差分と誤判定した: %q", line)
		}
	}
}
