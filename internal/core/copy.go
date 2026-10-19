package core

// copy.go - 進捗表示とコピー実行（bash 版 run_with_progress / copy_one の Go 移植）

import (
	"fmt"
	"io"
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
//
// 進捗は rsync の --out-format（%i %l %n）から、転送された通常ファイルの
// サイズを積み上げて求める。以前はコピー先を走査してバイト増分を測っていたが、
// 上書き更新では増分が 0 のままになり 100% に到達しないうえ、毎秒ツリー全体を
// 走査するため大きなコピーで負荷が高かった。
//
// Ctrl-C / TERM で子プロセスも確実に止め、終了コード 130 を返す。
func runWithProgress(cmd *exec.Cmd, j *Job) int {
	start := time.Now()

	// rsync の統計出力は一時ファイルへ退避し、進捗行と混ざらないようにする
	tmp, err := os.CreateTemp("", "safersync-*")
	if err != nil {
		return runPlain(cmd, j) // 一時ファイルが作れないときは進捗なし
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	cmd.Stdout = tmp
	cmd.Stderr = tmp

	counter := &transferredCounter{path: tmpName}

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
			_, done := counter.count()
			total := j.PlanBytes
			if total <= 0 {
				total = done
			}
			pct, _, speed, eta := progressFrom(done, 0, j.PlanBytes, start, time.Now())
			line := "  " + DrawProgress(pct, done, total, speed, eta)
			if j.ProgressTTY {
				// 端末では1行を書き換える（\r で行頭復帰 → 行末までクリア）。
				fmt.Fprintf(j.Out, "\r\033[K%s", line)
			} else {
				// 端末以外（--progress をパイプ/ログへ強制した場合）は制御文字を
				// 混入させず、そのまま1行として出す。
				fmt.Fprintln(j.Out, line)
			}
		}
	}

	if j.ProgressTTY {
		fmt.Fprint(j.Out, "\r\033[K")
	}
	// 子プロセスが終了してから書き込みを確定させる（開いたまま読むと
	// 末尾が欠ける環境がある）。
	_ = tmp.Close()
	data, _ := os.ReadFile(tmpName)
	if len(data) > 0 {
		// 進捗用のファイル一覧（--out-format）は表示せず、失敗時だけ原因究明の
		// ため stderr へ回す。--stats の統計行などは従来どおり stdout に出す。
		if interrupted || (rc != 0 && !IsVanishedExit(rc)) {
			_, _ = j.Err.Write(data)
		} else {
			// 成功時と rc=24（消えたファイル）は差分の羅列を出さず、統計行だけ出す。
			writeNonDiffOutput(j.Out, data)
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

// writeNonDiffOutput は rsync の --out-format 出力からファイルごとの差分行を
// 取り除き、--stats などの集計行だけを w へ書く。
func writeNonDiffOutput(w io.Writer, data []byte) {
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || isRsyncItemLine(line) {
			continue
		}
		fmt.Fprintln(w, line)
	}
}

// isRsyncItemLine は --out-format=%i の出力行（例: ">f+++++++++ 5 a.txt"）かを返す。
// 統計行（"Number of files: ..." など）と区別する。itemize の長さは実装で
// 異なる（GNU rsync は 11 文字、openrsync は 9 文字）ため、長さでは判定せず、
// 先頭2文字（更新種別・ファイル種別）と、その直後の区切りを見る。
func isRsyncItemLine(line string) bool {
	if len(line) < 3 {
		return false
	}
	if !strings.ContainsRune("<>ch.*", rune(line[0])) {
		return false
	}
	if !strings.ContainsRune("fdLDS", rune(line[1])) {
		return false
	}
	// item フィールドの後ろは必ず空白（%l などのため）。
	return strings.IndexByte(line, ' ') >= 2
}

// transferredCounter は rsync の --out-format=%i 出力を、前回読んだ位置から
// 追記分だけ走査して転送された通常ファイル数とバイト数を数える。毎tickで
// ファイル全体を読み直すのを避ける（転送ファイルが多いほど効く）。
type transferredCounter struct {
	path  string
	off   int64
	tail  string // 改行で終わっていない未完の行（次回に持ち越す）
	files int64
	bytes int64
	last  string // 最後に転送されたファイル名
}

// count は前回以降に追記された分だけを数え、累計のファイル数とバイト数を返す。
func (c *transferredCounter) count() (files, bytes int64) {
	f, err := os.Open(c.path)
	if err != nil {
		return c.files, c.bytes
	}
	defer f.Close()
	if _, err := f.Seek(c.off, io.SeekStart); err != nil {
		return c.files, c.bytes
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return c.files, c.bytes
	}
	c.off += int64(len(data))

	buf := c.tail + string(data)
	lines := strings.Split(buf, "\n")
	c.tail = lines[len(lines)-1] // 最後は未完の行かもしれない
	for _, line := range lines[:len(lines)-1] {
		if !strings.HasPrefix(line, ">f") {
			continue
		}
		c.files++
		item, size, name, ok := parseDiffLine(line)
		if ok {
			c.bytes += size
			if name != "" {
				c.last = name
			}
		} else if item == "" {
			continue
		}
	}
	return c.files, c.bytes
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
	if j.ListFiles || j.ShowProgress {
		// %i: itemize（">f" = 通常ファイル転送）、%l: ファイル長、%n: 名前。
		// 進捗は %l を積み上げ、TUI は %n を表示する。
		args = append(args, "--out-format=%i %l %n")
	}
	args = append(args, j.excludeArgs()...)
	if j.Checksum {
		args = append(args, "--checksum")
	}
	args = append(args, j.Src+"/", j.Dst+"/")
	switch {
	case j.Verbose:
		args = append(args, "--stats")
	case j.ListFiles || j.ShowProgress:
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
		// Ctrl-C / TERM などによる中断は、環境エラーではなく中断として返す
		if isInterrupted(plan.Err) {
			fmt.Fprintln(j.Err, "中断しました")
			return ExitInterrupted
		}
		// 差分が確認できないまま進むと「すべて最新です」と誤報告してしまう
		fmt.Fprintf(j.Err, "✗ 差分を確認できませんでした: %s\n", plan.Err)
		fmt.Fprintln(j.Err, "✗ rsync が使えるか、コピー先に接続できるかを確認してから再実行してください。")
		j.log(fmt.Sprintf("COPY abort  %s -> %s (diff unavailable)", j.Src, j.Dst))
		return ExitEnv
	}

	if plan.Changes == 0 {
		fmt.Fprintln(j.Out, "✓ 転送するものはありません（すべて最新です）")
		j.log(fmt.Sprintf("COPY skip   %s -> %s (no changes)", j.Src, j.Dst))
		return 0
	}

	// 進捗の分母に使う（総ファイルサイズではなく今回の転送量）。
	j.PlanBytes = plan.Bytes
	j.PlanFiles = plan.Files

	if j.DryRun {
		if plan.Files > 0 {
			fmt.Fprintf(j.Out, "    コピーするファイル: %d 件（%s） ※dry-run のため実際にはコピーしません\n",
				plan.Files, FmtBytes(plan.Bytes))
		}
		if plan.Files == 0 || plan.Changes != plan.Files {
			fmt.Fprintf(j.Out, "    実行が必要な変更: %d 件（シンボリックリンク・ディレクトリ・属性など） ※dry-run\n",
				plan.Changes)
		}
		// 見出しは「コピーするファイル」なので、詳細も通常ファイル（>f）だけを
		// 数える。ディレクトリ作成などは上の「実行が必要な変更」に含まれる。
		shown := 0
		for _, l := range plan.Lines {
			if !strings.HasPrefix(l, ">f") {
				continue
			}
			if shown >= 20 {
				break
			}
			fmt.Fprintf(j.Out, "    %s\n", l)
			shown++
		}
		if plan.Files > shown {
			fmt.Fprintf(j.Out, "    …（他 %d 件）\n", plan.Files-shown)
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
		free, ok := FreeBytesOK(j.Dst)
		switch {
		case !ok:
			fmt.Fprintln(j.Err, "! 空き容量を確認できませんでした（rsync に任せます）")
		case plan.Bytes > free:
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

	if plan.Files > 0 {
		fmt.Fprintf(j.Out, "    コピーするファイル: %d 件（%s）\n", plan.Files, FmtBytes(plan.Bytes))
	}
	if plan.Changes != plan.Files {
		fmt.Fprintf(j.Out, "    実行が必要な変更: %d 件（シンボリックリンク・ディレクトリ・属性など）\n", plan.Changes)
	}
	j.log(fmt.Sprintf("COPY start  %s -> %s (%d files, %d changes)", j.Src, j.Dst, plan.Files, plan.Changes))

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
	if IsVanishedExit(rc) {
		// 撮影中の SD カードなどで、走査後に消えたファイル。次の実行で拾えるため
		// 失敗扱いにしない（dry-run / 照合と同じ扱いにそろえる）。
		fmt.Fprintf(j.Err, "! 転送中に消えたファイルがありました（rc=%d、次回の実行で再確認されます）\n", rc)
		rc = 0
	}
	if rc != 0 {
		fmt.Fprintf(j.Err, "✗ rsync がエラー終了しました (rc=%d)\n", rc)
		j.log(fmt.Sprintf("COPY failed %s -> %s (rc=%d)", j.Src, j.Dst, rc))
		return 1
	}
	if plan.Files > 0 {
		fmt.Fprintf(j.Out, "✓ コピー完了（%d ファイル / %s / %s）\n",
			plan.Files, FmtBytes(plan.Bytes), FmtDuration(int64(time.Since(copyStart).Seconds())))
	} else {
		fmt.Fprintf(j.Out, "✓ コピー完了（変更 %d 件 / %s）\n",
			plan.Changes, FmtDuration(int64(time.Since(copyStart).Seconds())))
	}
	j.log(fmt.Sprintf("COPY done   %s -> %s", j.Src, j.Dst))
	j.TotalFiles += plan.Files
	j.TotalBytes += plan.Bytes
	return 0
}
