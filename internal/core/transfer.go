package core

// transfer.go - 転送の実行と進捗取得（TUI / 外部 UI 用）
// CLI 版は copy.go（runWithProgress）をそのまま使い、こちらは
// 「フレームを呼び出し側が自前で取る」スタイル（Bubble Tea から呼ぶ）。

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Transfer は走行中の rsync の1本分。
//
//	ルール:
//	- StartTransfer した goroutine 以外からも Frame / Interrupt を呼んでよい
//	  （Frame / CurrentFile / LogFile / Wait / Cleanup が共有するフィールドは
//	  mu で保護し、一時ファイルの Close は closeOnce で一度だけ行う）
//	- Wait は一度だけ呼ぶ（戻るまでブロック）
type Transfer struct {
	Job   *Job
	Total int64 // 進捗の分母（今回の転送量 PlanBytes）

	start    time.Time
	cmd      *exec.Cmd
	startErr error

	// rsync の --out-format 出力から進捗を数える（転送先の再走査はしない）。
	counter *transferredCounter

	// mu は進捗用 goroutine（Frame / CurrentFile）と終了処理（Wait /
	// Cleanup / LogFile）が別 goroutine から触るフィールドを保護する。
	// closeOnce は Wait と Cleanup が並行しても一時ファイルを二重 Close
	// しないためのもの。
	mu        sync.Mutex
	lastFile  string
	logFile   string
	log       *os.File
	closeOnce sync.Once
}

// StartTransfer は rsync を開始する（失敗時は startErr を返すので StartedOK で確認）。
// 進捗の分母は今回の転送量（PlanBytes）を使い、実績は rsync の出力（%l）から
// 積み上げる。転送先を走査しないため、上書き更新でも 100% まで到達する。
func (j *Job) StartTransfer() *Transfer {
	t := &Transfer{Job: j, start: time.Now(), Total: j.PlanBytes}
	// 転送中ファイル名を拾うために rsync の出力を一時ファイルへ退避する
	f, _ := os.CreateTemp("", "safersync-tui-*")
	if f != nil {
		t.log = f
		t.logFile = f.Name()
		t.counter = &transferredCounter{path: t.logFile}
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
// rsync の出力から転送済みバイト数を読み、転送先の再走査は行わない。
func (t *Transfer) Frame() (pct int, done, total, speed, eta int64) {
	if t.counter != nil {
		_, done = t.counter.count()
		if t.counter.last != "" {
			t.mu.Lock()
			t.lastFile = t.counter.last
			t.mu.Unlock()
		}
	}
	total = t.Total
	if total <= 0 {
		total = done
	}
	pct, done, speed, eta = progressFrom(done, 0, t.Total, t.start, time.Now())
	return pct, done, total, speed, eta
}

// closeLog は rsync 出力の一時ファイルを一度だけ閉じる。
// Wait と Cleanup のどちらから、どの順で呼ばれても二重 Close しない。
func (t *Transfer) closeLog() {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		f := t.log
		t.log = nil
		t.mu.Unlock()
		if f != nil {
			_ = f.Close()
		}
	})
}

// Wait は rsync の終了を待つ。戻り値は終了コード（0 / rsync の rc / 1）。
func (t *Transfer) Wait() int {
	if !t.StartedOK() {
		t.closeLog()
		return 1
	}
	err := t.cmd.Wait()
	t.closeLog()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}

// LogFile は rsync 出力の一時ファイルパスを返す。
func (t *Transfer) LogFile() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.logFile
}

// Cleanup は rsync 出力の一時ファイルを閉じて削除する。
// TUI の終了時や転送の再試行時に呼ぶ。Wait が呼ばれなかった場合でも
// ファイルディスクリプタを残さないよう、ここでも Close する。
// 二重に呼んでも実害はない。
func (t *Transfer) Cleanup() {
	t.closeLog()
	t.mu.Lock()
	path := t.logFile
	t.logFile = ""
	t.mu.Unlock()
	if path != "" {
		_ = os.Remove(path)
	}
}

// CurrentFile は rsync の出力から最後に処理したファイル名を返す（無ければ空）。
func (t *Transfer) CurrentFile() string {
	t.mu.Lock()
	name := t.lastFile
	path := t.logFile
	t.mu.Unlock()
	if name != "" {
		return name
	}
	if path == "" {
		return ""
	}
	if _, _, n, ok := parseDiffLine(lastLine(path)); ok {
		return n
	}
	return lastLine(path)
}

// lastLine は path の最後の空でない行を返す（無ければ空）。
// 進捗表示から毎秒呼ばれるため、ログ全体は読まず末尾から遡って読む。
func lastLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}

	const chunk = 64 << 10  // 一度に遡るサイズ
	const maxRead = 1 << 20 // 空行が続いても読みすぎない上限
	pos := fi.Size()
	var read int64
	buf := make([]byte, 0, chunk)
	for pos > 0 && read < maxRead {
		n := int64(chunk)
		if pos < n {
			n = pos
		}
		pos -= n
		read += n
		tmp := make([]byte, n)
		if _, err := f.ReadAt(tmp, pos); err != nil {
			return ""
		}
		buf = append(tmp, buf...)
		lines := bytes.Split(buf, []byte{'\n'})
		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}
			// buf の先頭行は途中で切れている可能性がある（読み残しがある）ため、
			// そこしか見つからない場合はさらに前を読む。
			if i == 0 && pos > 0 {
				break
			}
			return string(line)
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
