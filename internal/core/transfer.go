package core

// transfer.go - 転送の実行と進捗取得（TUI / 外部 UI 用）
// CLI 版は copy.go（runWithProgress）をそのまま使い、こちらは
// 「フレームを呼び出し側が自前で取る」スタイル（Bubble Tea から呼ぶ）。

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Transfer は走行中の rsync の1本分。
//
//	ルール:
//	- StartTransfer した goroutine 以外からも Frame / Interrupt を呼んでよい（内部は競合なし）
//	- Wait は一度だけ呼ぶ（戻るまでブロック）
type Transfer struct {
	Job   *Job
	Total int64 // src 全体のバイト数
	Base  int64 // src/転送開始時点の dst のバイト数

	start     time.Time
	cmd       *exec.Cmd
	startErr  error
	scanOnce  sync.Once
	scanTotal int64
	scanBase  int64

	logFile string
	log     *os.File
}

// StartTransfer は rsync を開始する（失敗時は startErr を返すので StartedOK で確認）。
func (j *Job) StartTransfer() *Transfer {
	t := &Transfer{Job: j, start: time.Now()}
	t.scanOnce.Do(func() {
		t.scanTotal = ScanDir(j.Src, j.AllExcludePatterns()).Bytes
		t.scanBase = ScanDir(j.Dst, j.AllExcludePatterns()).Bytes
	})
	t.Total, t.Base = t.scanTotal, t.scanBase
	// 転送中ファイル名を拾うために rsync の出力を一時ファイルへ退避する
	f, _ := os.CreateTemp("", "safersync-tui-*")
	if f != nil {
		t.log = f
		t.logFile = f.Name()
	}
	cmd := newRsyncCmd(j, j.rsyncArgs())
	if f != nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		t.startErr = err
		return t
	}
	t.cmd = cmd
	return t
}

// StartedOK は rsync が開始できたか。
func (t *Transfer) StartedOK() bool { return t != nil && t.startErr == nil && t.cmd != nil }

// StartError は開始失敗の原因。
func (t *Transfer) StartError() error { return t.startErr }

// Frame は現時点の進捗（bash 版 TUI と同じ: 1 秒間隔で呼ぶ想定）。
func (t *Transfer) Frame() (pct int, done, total, speed, eta int64) {
	p, done, speed, eta := scanProgress(t.Job, t.Total, t.Base, t.start, time.Now())
	return p, done, t.Total, speed, eta
}

// Wait は rsync の終了を待つ。戻り値は終了コード（0 / rsync の rc / 1）。
func (t *Transfer) Wait() int {
	if !t.StartedOK() {
		if t.log != nil {
			t.log.Close()
		}
		return 1
	}
	if err := t.cmd.Wait(); err != nil {
		if t.log != nil {
			t.log.Close()
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	if t.log != nil {
		t.log.Close()
	}
	return 0
}

// LogFile は rsync 出力の一時ファイルパスを返す。
func (t *Transfer) LogFile() string { return t.logFile }

// CurrentFile は rsync の出力から最後に処理したファイル名を返す（無ければ空）。
func (t *Transfer) CurrentFile() string {
	if t.logFile == "" {
		return ""
	}
	data, err := os.ReadFile(t.logFile)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

// Interrupt は子プロセス（caffeinate → rsync）へ TERM、1 秒後 KILL を送る。
// bash 版 tui_on_interrupt / run_with_progress と同じ挙動。
func (t *Transfer) Interrupt() {
	if !t.StartedOK() {
		return
	}
	killTree(t.cmd, syscall.SIGTERM)
	go func() {
		time.Sleep(1 * time.Second)
		killTree(t.cmd, syscall.SIGKILL)
	}()
}
