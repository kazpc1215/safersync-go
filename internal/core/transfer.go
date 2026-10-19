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
	Total int64 // 進捗の分母（開始時の dst バイト数 + 今回の転送量 PlanBytes）
	Base  int64 // src/転送開始時点の dst のバイト数

	start    time.Time
	cmd      *exec.Cmd
	startErr error

	// 非同期 dst 走査のキャッシュ（進捗をブロックしない）
	scanMu       sync.Mutex
	lastDstBytes int64
	scanning     bool

	logFile string
	log     *os.File
}

// StartTransfer は rsync を開始する（失敗時は startErr を返すので StartedOK で確認）。
// 進捗の分母はコピー元の走査（Job.SrcScan のキャッシュ）を、基準値はこの時点の
// 転送先の走査を使う。
func (j *Job) StartTransfer() *Transfer {
	t := &Transfer{Job: j, start: time.Now()}
	// 進捗の分母は「開始時の転送先バイト数 + 今回の転送量(PlanBytes)」。総ファイル
	// サイズを使うと差分コピーで開始直後から 100% になるため、今回の転送量を使う。
	t.Base = ScanDir(j.Dst, j.AllExcludePatterns()).Bytes
	t.Total = t.Base + j.PlanBytes
	t.lastDstBytes = t.Base
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
		// Wait を呼ぶ相手がいないため、ここで一時ファイルを片付ける
		if f != nil {
			f.Close()
			os.Remove(f.Name())
			t.log, t.logFile = nil, ""
		}
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
	// 非同期走査: 実行中なら前回値を使い、呼び出し側（UI/TUI）をブロックしない。
	t.scanMu.Lock()
	busy := t.scanning
	if !busy {
		t.scanning = true
		t.scanMu.Unlock()
		go func() {
			b := ScanDir(t.Job.Dst, t.Job.AllExcludePatterns()).Bytes
			t.scanMu.Lock()
			t.lastDstBytes = b
			t.scanning = false
			t.scanMu.Unlock()
		}()
	} else {
		t.scanMu.Unlock()
	}
	t.scanMu.Lock()
	db := t.lastDstBytes
	t.scanMu.Unlock()
	if db == 0 {
		db = t.Base
	}
	pct, done, speed, eta = progressFrom(db, t.Base, t.Job.PlanBytes, t.start, time.Now())
	return pct, done, t.Total, speed, eta
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

// Cleanup は rsync 出力の一時ファイルを削除する。開いたままでも削除でき
// （close された時点で消える）、TUI の終了時や転送の再試行時に呼ぶ。
// 二重に呼んでも実害はない。
func (t *Transfer) Cleanup() {
	if t.logFile != "" {
		os.Remove(t.logFile)
	}
}

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

// Terminate は子プロセスグループを止め、終了するまで待つ（最大 timeout、
// 終わらなければ KILL する）。TUI の終了時に呼び、rsync が孤児として
// コピーを続けないようにするための保険。
//
// Wait を別のゴルーチンが呼んでいても安全（自身は Wait を呼ばない）。
func (t *Transfer) Terminate(timeout time.Duration) {
	if !t.StartedOK() {
		return
	}
	killTree(t.cmd, syscall.SIGTERM)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !t.alive() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	killTree(t.cmd, syscall.SIGKILL)
}

// alive は子プロセスがまだ終了していないかを返す。
// Wait 済みのプロセスには Signal(0) が ErrProcessDone を返す。
func (t *Transfer) alive() bool {
	return t.cmd.Process.Signal(syscall.Signal(0)) == nil
}
