package core

// robustness_test.go - 重大バグの回帰テスト
//   * rsync を実行できないときに「差分なし / 照合 OK」と誤報告しない
//   * 進捗なし実行（runPlain）を中断しても rsync が孤児として残らない

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPlanChangesWithoutRsync(t *testing.T) {
	t.Setenv("PATH", "/nonexistent") // rsync を実行できない環境を再現

	j := &Job{Src: t.TempDir(), Dst: t.TempDir(), FAT: true}
	plan := j.PlanChanges()
	if plan.Err == nil {
		t.Fatal("rsync が実行できないのにエラーが返らない（「差分なし」と誤判定する）")
	}
	if plan.Files != 0 || plan.Bytes != 0 {
		t.Errorf("差分は 0 のはず: %d files / %d bytes", plan.Files, plan.Bytes)
	}
}

func TestVerifyOneWithoutRsync(t *testing.T) {
	t.Setenv("PATH", "/nonexistent") // rsync を実行できない環境を再現

	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rc := VerifyOne(&Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errOut})
	if rc != ExitEnv {
		t.Fatalf("want exit %d, got %d (stderr=%q)", ExitEnv, rc, errOut.String())
	}
	if strings.Contains(out.String(), "照合 OK") {
		t.Error("照合できていないのに「照合 OK」と表示された（コピー元を消してよいと誤解する）")
	}
	if !strings.Contains(errOut.String(), "照合できませんでした") {
		t.Errorf("照合できなかった旨のメッセージが無い: %q", errOut.String())
	}
}

func TestCopyOneWithoutRsync(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	rc := CopyOne(&Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errOut})
	if rc != ExitEnv {
		t.Fatalf("want exit %d, got %d (stderr=%q)", ExitEnv, rc, errOut.String())
	}
	if strings.Contains(out.String(), "すべて最新") {
		t.Error("差分を確認できていないのに「すべて最新です」と表示された")
	}
}

func TestRunPlainReturnsExitCode(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 3")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out, errOut bytes.Buffer
	if rc := runPlain(cmd, &Job{Out: &out, Err: &errOut}); rc != 3 {
		t.Fatalf("want 3, got %d", rc)
	}
}

func TestRunPlainInterruptKillsChild(t *testing.T) {
	// 子プロセスは本番と同じく別プロセスグループで起動する。
	// 起動検知はマーカーファイルで行う（cmd.Process を外から読むとデータ競合になる）。
	const sentinel = "3141592"
	marker := filepath.Join(t.TempDir(), "started")
	cmd := exec.Command("sh", "-c", `touch "$1" && exec sleep `+sentinel, "sh", marker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out, errOut bytes.Buffer

	done := make(chan int, 1)
	go func() { done <- runPlain(cmd, &Job{Out: &out, Err: &errOut}) }()

	deadline := time.Now().Add(5 * time.Second)
	for !fileExists(marker) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !fileExists(marker) {
		t.Fatal("子プロセスが起動しない")
	}

	// Ctrl-C 相当（自分自身へ SIGINT）。runPlain が受け取り、子ごと止める
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}

	select {
	case rc := <-done:
		if rc != ExitInterrupted {
			t.Fatalf("want %d, got %d", ExitInterrupted, rc)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runPlain が中断されない")
	}

	// 子が孤児として残っていないこと
	for i := 0; i < 20; i++ {
		if exec.Command("pgrep", "-f", "sleep "+sentinel).Run() != nil {
			return // 残っていない
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = exec.Command("pkill", "-f", "sleep "+sentinel).Run()
	t.Fatal("中断後も子プロセスが残っている（rsync の孤児化）")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestTransferTerminateKillsChild(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	tr := &Transfer{cmd: cmd}
	go func() { _ = cmd.Wait() }() // 本番同様、別ゴルーチンが Wait する

	start := time.Now()
	tr.Terminate(2 * time.Second)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Terminate が終わらない: %s", elapsed)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("Terminate 後も子プロセスが残っている")
	}
}

// 事前にキャンセルされたコンテキストでの差分スキャンは、環境エラーではなく
// 中断として扱われること（終了コード 130）。
func TestCopyOneInterruptedDiff(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync が見つからないためスキップ")
	}
	// 転送対象を1つ置き、rsync が確実に差分を返す状況にする。
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // スキャン前に中断

	var out, errOut bytes.Buffer
	j := &Job{Src: src, Dst: dst, FAT: true, Ctx: ctx, Out: &out, Err: &errOut}
	if rc := CopyOne(j); rc != ExitInterrupted {
		t.Fatalf("want exit %d, got %d (stdout=%q stderr=%q)", ExitInterrupted, rc, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "中断しました") {
		t.Errorf("中断メッセージが出ていない: %q", errOut.String())
	}
}

func TestIsInterrupted(t *testing.T) {
	if !isInterrupted(context.Canceled) {
		t.Error("context.Canceled を中断と判定していない")
	}
	if !isInterrupted(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded を中断と判定していない")
	}
	if isInterrupted(nil) {
		t.Error("nil を中断と誤判定している")
	}
	if isInterrupted(errors.New("rsync がエラー終了しました (rc=23)")) {
		t.Error("通常のエラーを中断と誤判定している")
	}
}
