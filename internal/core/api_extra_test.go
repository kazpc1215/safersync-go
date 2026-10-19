package core

// api_extra_test.go - これまで未到達だった入口のテスト:
//   NewOptions / firstLine / RunJob / Transfer.Frame / Transfer.Interrupt
// rsync が要る経路は RsyncAvailable でスキップする（CI には有る）。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNewOptionsDefaults(t *testing.T) {
	o := NewOptions()
	if o == nil {
		t.Fatal("NewOptions が nil を返した")
	}
	if !o.FAT {
		t.Error("既定は FAT（exFAT 安全側）のはず")
	}
	if o.APFS {
		t.Error("既定で APFS が有効になっている")
	}
}

func TestFirstLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"\n\n", ""},
		{"  \n\nerr: boom\nmore", "err: boom"},
		{"  leading\n", "leading"},
	}
	for _, tc := range cases {
		if got := firstLine(tc.in); got != tc.want {
			t.Errorf("firstLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRunJobVerifyOnly(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	j := &Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errb}
	if rc := RunJob(j, true, false, false); rc != 0 {
		t.Errorf("verify-only は 0 を返すはず: rc=%d (stderr=%q)", rc, errb.String())
	}
}

func TestRunJobCopyDryRun(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	j := &Job{Src: src, Dst: dst, FAT: true, DryRun: true, Out: &out, Err: &errb}
	if rc := RunJob(j, false, true, false); rc != 0 {
		t.Errorf("dry-run はコピー後に 0 を返すはず: rc=%d (stderr=%q)", rc, errb.String())
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err == nil {
		t.Error("dry-run なのにコピー先へ書き込まれている")
	}
}

func TestRunJobCopyAndVerify(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	j := &Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errb}
	if rc := RunJob(j, false, false, true); rc != 0 {
		t.Errorf("コピー + 照合は 0 を返すはず: rc=%d (stderr=%q)", rc, errb.String())
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Errorf("コピーされていない: %v", err)
	}
}

// Frame は rsync 起動前でも安全に呼べる（分母は Total）。
func TestTransferFrameBeforeStart(t *testing.T) {
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := &Transfer{Job: &Job{Dst: dst, FAT: true, ListFiles: true}, Total: 100}
	for i := 0; i < 3; i++ {
		pct, _, total, _, _ := tr.Frame()
		if total != 100 {
			t.Fatalf("Total をそのまま返すはず: got %d", total)
		}
		if pct < 0 || pct > 100 {
			t.Fatalf("進捗率が範囲外: %d", pct)
		}
	}
}

func TestTransferInterrupt(t *testing.T) {
	// 未起動（cmd なし）の Transfer には何もしない（nil でも安全）
	var nilTr *Transfer
	nilTr.Interrupt()
	(&Transfer{}).Interrupt()

	// 実行中の子プロセスには TERM を送り、1 秒後の KILL で確実に止まる
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tr := &Transfer{cmd: cmd}
	tr.Interrupt()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("Interrupt 後も子プロセスが終了しない")
	}
}
