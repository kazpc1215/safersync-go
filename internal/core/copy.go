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
	"sync"
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

// DrawProgressFiles はリモート先など転送量（バイト）をローカルで数えられない
// ときの進捗1行分（件数ベース）。速度は「件/秒」になる。
func DrawProgressFiles(pct int, done, total int64, speed float64, eta int64) string {
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
	fmt.Fprintf(&b, "%3d%%  %d / %d ファイル  %.1f件/秒%s", pct, done, total, speed, tail)
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
// 回収済み（Wait 済み）のプロセスへ送ると、PID が再利用されていた場合に
// 無関係なプロセスグループを止めてしまうため、生存を確認してから送る。
func killTree(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Signal(0) は Wait で回収済みなら ErrProcessDone を返す（os.Process 内部で
	// 同期されている）。これで遅延 KILL（1秒後）等が無関係な PID を撃つのを防ぐ。
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
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

// runWithProgress は cmd を実行しながら進捗と残り時間を表示する。
//   - ローカル: 転送先の増分バイト数から計算（分母は「開始時の転送先 + 今回の転送量」）。
//   - リモート: 転送先をローカル走査できないため、rsync が出力する転送ファイルの
//     印（--out-format=%i の ">f..."）の件数から計算する。
//
// Ctrl-C / TERM で子プロセスも確実に止め、終了コード 130 を返す。
func runWithProgress(cmd *exec.Cmd, j *Job) int {
	// 転送量はコピー先のバイト増分で測るため、コピー先がローカルでなければ
	// バイト走査ができない（コピー元のリモート可否は無関係）。
	remote := IsRemote(j.Dst)
	start := time.Now()

	// ローカルのみ: 開始時点の転送先バイト数を基準に、分母を「基準 + 今回の転送量」にする。
	var base, total int64
	if !remote {
		base = ScanDir(j.Dst, j.AllExcludePatterns()).Bytes
		total = base + j.PlanBytes
	}

	// 走査は別 goroutine で行い、シグナル受付・描画をブロックしない（大きなツリー対策）。
	// 直前の走査結果がまだ無ければ base を使う。
	var scanMu sync.Mutex
	lastDstBytes := base
	scanning := false

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
			if remote {
				files := countTransferredFiles(tmp.Name())
				p, speed, eta := progressCountFrom(files, int64(j.PlanFiles), start, time.Now())
				fmt.Fprintf(j.Out, "\r\033[K  %s",
					DrawProgressFiles(p, files, int64(j.PlanFiles), speed, eta))
				continue
			}
			// 非同期走査: 実行中なら前回値を使い、UI/シグナル処理を止めない
			scanMu.Lock()
			busy := scanning
			if !busy {
				scanning = true
				scanMu.Unlock()
				go func() {
					b := ScanDir(j.Dst, j.AllExcludePatterns()).Bytes
					scanMu.Lock()
					lastDstBytes = b
					scanning = false
					scanMu.Unlock()
				}()
			} else {
				scanMu.Unlock()
			}
			scanMu.Lock()
			db := lastDstBytes
			scanMu.Unlock()
			if db == 0 {
				db = base
			}
			p, doneBytes, speed, eta := progressFrom(db, base, j.PlanBytes, start, time.Now())
			fmt.Fprintf(j.Out, "\r\033[K  %s", DrawProgress(p, doneBytes, total, speed, eta))
		}
	}

	fmt.Fprint(j.Out, "\r\033[K")
	if data, _ := os.ReadFile(tmp.Name()); len(data) > 0 {
		// リモートの進捗用出力（--out-format=%i）はファイル名の羅列なので、
		// 成功時は表示しない。失敗時のみ原因究明のため stderr へ回す。
		if !remote {
			_, _ = j.Out.Write(data)
		} else if rc != 0 || interrupted {
			_, _ = j.Err.Write(data)
		}
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

// countTransferredFiles は rsync の --out-format=%i 出力から、転送された通常
// ファイルの行数（行頭が ">f"）を数える。ディレクトリ作成（"cd..."）や
// 属性のみの変化（".f..."）は数えない。
func countTransferredFiles(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var n int64
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, ">f") {
			n++
		}
	}
	return n
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
	switch {
	case j.ListFiles:
		args = append(args, "--out-format=%n")
	case j.ProgressFiles:
		// リモート先はローカルでバイトを数えられないため、転送したファイルの
		// 印（%i の ">f..."）を行数として数える。ディレクトリ作成（"cd..."）は
		// ">" で始まらないので数えない。
		args = append(args, "--out-format=%i")
	}
	args = append(args, j.excludeArgs()...)
	args = append(args, j.Src+"/", j.Dst+"/")
	switch {
	case j.Verbose:
		args = append(args, "--stats")
	case j.ListFiles || j.ProgressFiles:
		// -q は --out-format の出力（進捗の数え上げ・TUI のファイル名表示）も
		// 抑制してしまうため、出力を使うときは付けない（エラーは stderr に出る）。
	default:
		args = append(args, "-q")
	}
	return args
}

const progressInterval = 2 * time.Second

// progressFrom は走査済みの dst バイト数から進捗を計算する（走査そのものは含まない）。
// 非同期走査（runWithProgress / Transfer.Frame）から使う。
//
// 分母は「開始時の転送先(base) + 今回の転送量(planBytes)」。総ファイルサイズでは
// なく今回の転送量を基準にするため、差分コピー（再開・追記）でも進捗が 0% から
// 100% まで動く。planBytes が不明（<=0）のときは従来どおり 100% 扱い。
func progressFrom(doneBytes, base, planBytes int64, start time.Time, now time.Time) (p int, done, speed, eta int64) {
	elapsed := int64(now.Sub(start).Seconds())
	if elapsed < 1 {
		elapsed = 1
	}
	gained := doneBytes - base
	if gained < 0 {
		gained = 0
	}
	speed = gained / elapsed
	p = 100
	if planBytes > 0 {
		p = int(gained * 100 / planBytes)
	}
	if p > 100 {
		p = 100
	}
	if p < 0 {
		p = 0
	}
	eta = -1
	if speed > 0 && planBytes > gained {
		eta = (planBytes - gained) / speed
	}
	return p, doneBytes, speed, eta
}

// progressCountFrom は転送済みファイル数から進捗を計算する（件数ベース）。
// リモート先など、転送量（バイト）をローカルで数えられない場合に使う。
// 速度は件/秒（小数）で返す。
func progressCountFrom(doneFiles, planFiles int64, start time.Time, now time.Time) (p int, speed float64, eta int64) {
	elapsed := int64(now.Sub(start).Seconds())
	if elapsed < 1 {
		elapsed = 1
	}
	if doneFiles < 0 {
		doneFiles = 0
	}
	p = 100
	if planFiles > 0 {
		p = int(doneFiles * 100 / planFiles)
	}
	if p > 100 {
		p = 100
	}
	speed = float64(doneFiles) / float64(elapsed)
	eta = -1
	if speed > 0 && planFiles > doneFiles {
		eta = int64(float64(planFiles-doneFiles) / speed)
	}
	return p, speed, eta
}

// ensureLocalDest はローカルのコピー先フォルダを作る。リモート先（host:path）
// はローカルに同名のディレクトリを作ってしまう（"user@host:" という迷子
// フォルダ）ため何もしない（作成は rsync が行う）。
func ensureLocalDest(dst string) error {
	if IsRemote(dst) {
		return nil
	}
	return os.MkdirAll(dst, 0o755)
}

// CopyOne は1組のコピーを実行する。戻り値は終了コード（0 / 1 / 3 / 130）。
func CopyOne(j *Job) int {
	// ローカルのコピー元だけ事前に確認する。リモート元はローカルから stat できず、
	// ここで弾くと「リモート元が使えない」ことになる（存在確認は rsync が行う）。
	if !IsRemote(j.Src) {
		if fi, err := os.Stat(j.Src); err != nil || !fi.IsDir() {
			fmt.Fprintf(j.Err, "✗ コピー元が存在しません: %s\n", j.Src)
			return 1
		}
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

	// 進捗の分母に使う（総ファイルサイズではなく今回の転送量）。
	j.PlanBytes = plan.Bytes
	j.PlanFiles = plan.Files

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

	// 空き容量の確認（足りなければ、始める前に止める）。
	// リモート先の空き容量はローカルから確認できないため、その旨だけ知らせる。
	switch {
	case j.IgnoreSpace:
	case IsRemote(j.Dst):
		fmt.Fprintln(j.Out, "    リモート先の空き容量は確認できません（rsync に任せます）")
	default:
		if free := FreeBytes(j.Dst); free > 0 && plan.Bytes > free {
			fmt.Fprintf(j.Err, "✗ 空き容量が足りません: コピーする量 %s / 空き %s\n",
				FmtBytes(plan.Bytes), FmtBytes(free))
			fmt.Fprintln(j.Err, "✗ 不要なファイルを削除するか、別の保存先を指定してください。")
			fmt.Fprintln(j.Err, "✗ 空き容量を無視して続行するには --ignore-space-check を付けてください。")
			j.log(fmt.Sprintf("COPY abort  %s -> %s (not enough space)", j.Src, j.Dst))
			return ExitEnv
		}
	}

	// ローカルのコピー先だけ作成する。リモート先はローカルにディレクトリを
	// 作ってしまう（"user@host:" という迷子フォルダ）ので触らない。
	if err := ensureLocalDest(j.Dst); err != nil {
		fmt.Fprintf(j.Err, "✗ コピー先を作成できません: %s\n", j.Dst)
		return 1
	}

	// リモート先の進捗はバイト数をローカルで数えられないため、ファイルの印を
	// 数えるモードに切り替える（rsync 引数の組み立て前に設定する）。
	if j.ShowProgress && IsRemote(j.Dst) {
		j.ProgressFiles = true
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
