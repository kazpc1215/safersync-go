package core

// copy.go - 進捗表示とコピー実行（bash 版 run_with_progress / copy_one の Go 移植）

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// 進捗表示（bash 版 draw_progress と同じ書式）
// ---------------------------------------------------------------------------

// progressWidth は端末幅から進捗バーの長さを決める（狭いと折り返して崩れるため）。
func progressWidth() int {
	cols := 80
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cols = n
		}
	}
	switch {
	case cols >= 110:
		return 24
	case cols >= 90:
		return 16
	case cols >= 70:
		return 8
	default:
		return 0
	}
}

// DrawProgress は進捗1行分の文字列を返す。
// 先頭の "\r\033[K"（行頭復帰）は呼び出し側が付ける。
func DrawProgress(pct int, done, total, speed, eta int64) string {
	var b strings.Builder
	if width := progressWidth(); width > 0 {
		filled := pct * width / 100
		bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
		b.WriteString("[" + bar + "] ")
	}
	etaTxt := "--"
	if eta >= 0 {
		etaTxt = "約 " + FmtDuration(eta)
	}
	tail := "  残り " + etaTxt
	if pct >= 100 {
		tail = ""
	}
	fmt.Fprintf(&b, "%3d%%  %s / %s  %s/s%s",
		pct, FmtBytes(done), FmtBytes(total), FmtBytes(speed), tail)
	return b.String()
}

// ---------------------------------------------------------------------------
// rsync 実行ヘルパ
// ---------------------------------------------------------------------------

// newRsyncCmd は rsync（必要なら caffeinate 経由）の実行コマンドを組み立てる。
// 中断時に子プロセスグループごと止められるよう、プロセスグループを分離する。
func newRsyncCmd(j *Job, args []string) *exec.Cmd {
	var cmd *exec.Cmd
	if !j.NoCaffeinate && runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("caffeinate"); err == nil {
			cmd = exec.Command("caffeinate", append([]string{"-i", "rsync"}, args...)...)
		}
	}
	if cmd == nil {
		cmd = exec.Command("rsync", args...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// killTree は子プロセス（caffeinate → rsync）グループへシグナルを送る。
func killTree(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
	_ = syscall.Kill(cmd.Process.Pid, sig)
}

// watchSignals は中断系シグナル（INT / TERM / HUP）の受信を始める。
// 子プロセスは別プロセスグループで動くため、シグナルを受け取る前に起動すると
// 子だけが残ってしまう。必ず cmd.Start の前に呼ぶこと。
func watchSignals() (chan os.Signal, func()) {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return ch, func() { signal.Stop(ch) }
}

// terminateChild は子プロセスグループへ TERM を送り、1 秒後にも残っていれば
// KILL する（bash 版 run_with_progress の trap と同じ挙動）。
func terminateChild(cmd *exec.Cmd) {
	killTree(cmd, syscall.SIGTERM)
	go func() {
		time.Sleep(1 * time.Second)
		killTree(cmd, syscall.SIGKILL)
	}()
}

// runPlain は進捗なしで cmd を実行する。stdout/stderr は Job のOut/Errへ直結する。
// Ctrl-C / TERM / HUP では子プロセスごと確実に止め、130 を返す。
func runPlain(cmd *exec.Cmd, j *Job) int {
	cmd.Stdout = j.Out
	cmd.Stderr = j.Err

	sigCh, stop := watchSignals()
	defer stop()

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(j.Err, "✗ 実行できませんでした: %s\n", err)
		return 1
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	for {
		select {
		case err := <-waitDone:
			return exitCodeOf(err)
		case <-sigCh:
			terminateChild(cmd)
			// 追加のシグナルが来たら、待たずに強制終了する
			for {
				select {
				case <-waitDone:
					fmt.Fprintln(j.Err, "中断しました")
					return ExitInterrupted
				case <-sigCh:
					killTree(cmd, syscall.SIGKILL)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 進捗つき実行（bash 版 run_with_progress）
// ---------------------------------------------------------------------------

// runWithProgress は cmd を実行しながら、転送先の増分から進捗と残り時間を表示する。
// Ctrl-C / TERM で子プロセスも確実に止め、終了コード 130 を返す。
func runWithProgress(cmd *exec.Cmd, j *Job) int {
	total := ScanDir(j.Src, j.AllExcludePatterns()).Bytes
	base := ScanDir(j.Dst, j.AllExcludePatterns()).Bytes
	start := time.Now()

	// rsync の統計出力は一時ファイルへ退避し、進捗行と混ざらないようにする
	tmp, err := os.CreateTemp("", "safersync-*")
	if err != nil {
		return runPlain(cmd, j) // 一時ファイルが作れないときは進捗なし
	}
	defer os.Remove(tmp.Name())
	cmd.Stdout = tmp
	cmd.Stderr = tmp

	// 中断（Ctrl-C / TERM / HUP）の受付。起動前に登録して取りこぼしを防ぐ
	sigCh, stop := watchSignals()
	defer stop()
	interrupted := false

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(j.Err, "✗ 実行できませんでした: %s\n", err)
		return 1
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()

	var rc int
loop:
	for {
		select {
		case err := <-waitDone:
			rc = exitCodeOf(err)
			break loop
		case <-sigCh:
			if !interrupted {
				interrupted = true
				terminateChild(cmd)
			} else {
				killTree(cmd, syscall.SIGKILL) // 2回目以降は即時
			}
		case <-ticker.C:
			if interrupted {
				continue
			}
			p, doneBytes, speed, eta := scanProgress(j, total, base, start, time.Now())
			fmt.Fprintf(j.Out, "\r\033[K  %s", DrawProgress(p, doneBytes, total, speed, eta))
		}
	}

	fmt.Fprint(j.Out, "\r\033[K")
	if data, _ := os.ReadFile(tmp.Name()); len(data) > 0 {
		_, _ = j.Out.Write(data)
	}
	if interrupted {
		fmt.Fprintln(j.Err, "中断しました")
		return ExitInterrupted
	}
	if rc != 0 {
		return rc
	}
	return 0
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}

// ---------------------------------------------------------------------------
// コピー本体（bash 版 copy_one。「--delete は使わない。既存のファイルは消さない」）
// ---------------------------------------------------------------------------

// rsyncArgs はコピー実行用の rsync 引数を組み立てる（--human-readable --partial まで）。
func (j *Job) rsyncArgs() []string {
	args := append([]string{}, j.rsyncMetaOpts()...)
	args = append(args, "--human-readable", "--partial")
	if j.ListFiles {
		args = append(args, "--out-format=%n")
	}
	args = append(args, j.excludeArgs()...)
	args = append(args, j.Src+"/", j.Dst+"/")
	switch {
	case j.Verbose:
		args = append(args, "--stats")
	case j.ListFiles:
		// -q は --out-format の出力（TUI のファイル名表示）も抑制してしまうため、
		// ファイル名を出すときは付けない（エラーは stderr に出る）。
	default:
		args = append(args, "-q")
	}
	return args
}

const progressInterval = 2 * time.Second

// scanProgress は転送先の累計バイト数から進捗を計算する（bash 版 run_with_progress / TUI 版共通）。
func scanProgress(watched *Job, total, base int64, start time.Time, now time.Time) (p int, done, speed, eta int64) {
	elapsed := int64(now.Sub(start).Seconds())
	if elapsed < 1 {
		elapsed = 1
	}
	doneBytes := ScanDir(watched.Dst, watched.AllExcludePatterns()).Bytes
	gained := doneBytes - base
	if gained < 0 {
		gained = 0
	}
	speed = gained / elapsed
	p = 100
	if total > 0 {
		p = int(doneBytes * 100 / total)
		if p > 100 {
			p = 100
		}
	}
	eta = -1
	if speed > 0 && total > doneBytes {
		eta = (total - doneBytes) / speed
	}
	return p, doneBytes, speed, eta
}

// CopyOne は1組のコピーを実行する。戻り値は終了コード（0 / 1 / 3 / 130）。
func CopyOne(j *Job) int {
	if fi, err := os.Stat(j.Src); err != nil || !fi.IsDir() {
		fmt.Fprintf(j.Err, "✗ コピー元が存在しません: %s\n", j.Src)
		return 1
	}

	// ボリュームの直下を指定すると中身が散らばるので、先に知らせる（ローカルのみ）
	if !IsRemote(j.Dst) && IsMountPoint(j.Dst) {
		fmt.Fprintf(j.Err, "! コピー先がボリュームの直下です: %s\n", j.Dst)
		fmt.Fprintf(j.Err, "! コピー元の中身がこのフォルダの直下に入ります（まとめたいときは下位フォルダを指定してください）\n")
	}

	// 何件・何バイトコピーする必要があるかを先に調べる
	plan := j.PlanChanges()
	if plan.Err != nil {
		// 差分が確認できないまま進むと「すべて最新です」と誤報告してしまう
		fmt.Fprintf(j.Err, "✗ 差分を確認できませんでした: %s\n", plan.Err)
		fmt.Fprintln(j.Err, "✗ rsync が使えるか、コピー先に接続できるかを確認してから再実行してください。")
		j.log(fmt.Sprintf("COPY abort  %s -> %s (diff unavailable)", j.Src, j.Dst))
		return ExitEnv
	}

	if plan.Files == 0 {
		fmt.Fprintln(j.Out, "✓ 転送するものはありません（すべて最新です）")
		j.log(fmt.Sprintf("COPY skip   %s -> %s (no changes)", j.Src, j.Dst))
		return 0
	}

	if j.DryRun {
		fmt.Fprintf(j.Out, "    コピーするファイル: %d 件（%s） ※dry-run のため実際にはコピーしません\n",
			plan.Files, FmtBytes(plan.Bytes))
		shown := 0
		for _, l := range plan.Lines {
			if shown >= 20 {
				break
			}
			fmt.Fprintf(j.Out, "    %s\n", l)
			shown++
		}
		if len(plan.Lines) > 20 {
			fmt.Fprintf(j.Out, "    …（他 %d 件）\n", len(plan.Lines)-20)
		}
		fmt.Fprintln(j.Out, "    （記号: > 新規/更新, f ファイル, + 新規作成, s サイズ, t 時刻）")
		return 0
	}

	// 空き容量の確認（足りなければ、始める前に止める）
	if !j.IgnoreSpace {
		free := FreeBytes(j.Dst)
		if free > 0 && plan.Bytes > free {
			fmt.Fprintf(j.Err, "✗ 空き容量が足りません: コピーする量 %s / 空き %s\n",
				FmtBytes(plan.Bytes), FmtBytes(free))
			fmt.Fprintln(j.Err, "✗ 不要なファイルを削除するか、別の保存先を指定してください。")
			fmt.Fprintln(j.Err, "✗ 空き容量を無視して続行するには --ignore-space-check を付けてください。")
			j.log(fmt.Sprintf("COPY abort  %s -> %s (not enough space)", j.Src, j.Dst))
			return ExitEnv
		}
	}

	if err := os.MkdirAll(j.Dst, 0o755); err != nil {
		fmt.Fprintf(j.Err, "✗ コピー先を作成できません: %s\n", j.Dst)
		return 1
	}

	fmt.Fprintf(j.Out, "    コピーするファイル: %d 件（%s）\n", plan.Files, FmtBytes(plan.Bytes))
	j.log(fmt.Sprintf("COPY start  %s -> %s (%d files)", j.Src, j.Dst, plan.Files))

	cmd := newRsyncCmd(j, j.rsyncArgs())

	copyStart := time.Now()
	var rc int
	if j.ShowProgress {
		rc = runWithProgress(cmd, j)
	} else {
		rc = runPlain(cmd, j)
	}
	if rc == ExitInterrupted {
		return ExitInterrupted
	}
	if rc != 0 {
		fmt.Fprintf(j.Err, "✗ rsync がエラー終了しました (rc=%d)\n", rc)
		j.log(fmt.Sprintf("COPY failed %s -> %s (rc=%d)", j.Src, j.Dst, rc))
		return 1
	}
	fmt.Fprintf(j.Out, "✓ コピー完了（%d ファイル / %s / %s）\n",
		plan.Files, FmtBytes(plan.Bytes), FmtDuration(int64(time.Since(copyStart).Seconds())))
	j.log(fmt.Sprintf("COPY done   %s -> %s", j.Src, j.Dst))
	j.TotalFiles += plan.Files
	j.TotalBytes += plan.Bytes
	return 0
}
