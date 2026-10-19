package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/kazpc1215/safersync-go/internal/core"
	"github.com/kazpc1215/safersync-go/internal/tui"
	"github.com/kazpc1215/safersync-go/internal/version"
)

// colors は端末のときだけ使う ANSI 色。非端末では全て空にする（bash 版と同じ）。
type colors struct{ bold, red, green, yellow, blue, reset string }

func newColors(stdout io.Writer) colors {
	if !isTerminalWriter(stdout) {
		return colors{}
	}
	return colors{bold: "\x1b[1m", red: "\x1b[31m", green: "\x1b[32m",
		yellow: "\x1b[33m", blue: "\x1b[34m", reset: "\x1b[0m"}
}

// isTTY は f が端末かどうかを返す。
// キャラクタデバイス判定だけだと /dev/null も端末と誤認し、
// 「TUI は対話端末でのみ」チェックをすり抜けてしまうため、端末の ioctl で判定する。
func isTTY(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// isTerminalWriter / isTerminalReader は、注入された入出力が端末かどうかを返す。
// *os.File 以外（テストのバッファなど）は端末ではない扱い。Run は writer を
// 注入できる契約なので、端末判定も os.Stdout 固定にはせず渡された値で行う。
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && isTTY(f)
}

func isTerminalReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && isTTY(f)
}

// Run は safersync の本体。bash 版 main() と同じ流れ・出力・終了コード。
// 標準入力は TUI 起動可否の判定と TUI 本体にだけ使う。
func Run(argv []string, stdout, stderr io.Writer) int {
	return run(argv, os.Stdin, stdout, stderr)
}

// run は Run の実体（stdin を注入できる版。テスト用）。
func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	noArgs := len(argv) == 0

	opts, err := Parse(argv)
	if err != nil {
		fmt.Fprintf(stderr, "✗ %s\n", err)
		return ExitError
	}

	switch {
	case opts.ShowVersion:
		fmt.Fprintf(stdout, "safersync v%s\n", version.Version)
		return ExitOK
	case opts.ShowHelp:
		printHelp(stdout, core.LogFile())
		return ExitOK
	}

	start := time.Now()
	c := newColors(stdout)

	if opts.ShowStatus {
		return doStatus(opts, c, stdout)
	}

	// rsync はこのツールの実行に必須。無いまま進むと「差分なし」「照合 OK」と
	// 誤って成功を報告してしまうため、開始前に止める（終了コード 3）。
	if !core.RsyncAvailable() {
		fmt.Fprintln(stderr, "✗ rsync が見つかりません（macOS / Linux には標準搭載されています）")
		fmt.Fprintln(stderr, "✗ rsync をインストールしてから再実行してください。")
		return ExitEnv
	}

	// --- 対話ウィザード（TUI）---
	//   引数なしで対話端末なら自動起動。--tui/-i なら明示的に起動する。
	//   端末判定は注入された入出力（テストではバッファ）で行う。
	inTTY, outTTY := isTerminalReader(stdin), isTerminalWriter(stdout)
	if opts.TUI || (noArgs && inTTY && outTTY) {
		if !inTTY || !outTTY {
			fmt.Fprintln(stderr, "✗ TUI は対話端末でのみ起動できます（--help を参照）")
			return ExitError
		}
		if len(opts.Jobs) > 0 {
			fmt.Fprintln(stderr, "✗ TUI では --job は使えません（位置引数か --from / --to で1組だけ指定してください）")
			return ExitError
		}
		// 対話ウィザード（Bubble Tea）。戻り値は CLI と同じ終了コード体系
		return tui.RunTui(opts, mountRootOf(opts), stdin, stdout)
	}

	// --- 実行対象ジョブの決定（単発 or 複数）---
	type pair struct{ src, dst string }
	var pairs []pair
	if len(opts.Jobs) > 0 {
		for _, j := range opts.Jobs {
			pairs = append(pairs, pair{j.From, j.To})
		}
	} else {
		if opts.From == "" || opts.To == "" {
			fmt.Fprintln(stderr, "✗ コピー元とコピー先を指定してください（例: safersync ~/書類 /Volumes/HDD/書類）")
			return ExitError
		}
		pairs = append(pairs, pair{opts.From, opts.To})
	}

	lastRC := 0
	// 最初の差分スキャン（dry-run の rsync）は、コピー本体（runPlain /
	// runWithProgress）より前に走る。ここでも Ctrl-C / TERM / HUP を効かせ、
	// 大きなツリーのスキャン中に中断できるようにする（exec.CommandContext）。
	ctx, stopSignals := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stopSignals()
	for idx, p := range pairs {
		// 前のジョブで Ctrl-C / TERM を受けていたら、次のジョブは始めない
		// （中断したのに次のジョブを走らせると、利用者の意図に反してコピーが続く）。
		if jobLoopStop(ctx, lastRC) {
			if lastRC == 0 {
				lastRC = ExitInterrupted
			}
			break
		}
		if len(pairs) > 1 {
			fmt.Fprintf(stdout, "\n%s=== ジョブ %d/%d ===%s\n", c.bold, idx+1, len(pairs), c.reset)
		}

		src := p.src
		dst := p.dst
		// ローカルの名前に ":" を含み、かつ実在する場合は「リモート誤認」の
		// ヒントを先に出す（rsync はホスト名解決の失敗で分かりにくく終わるため）。
		for _, cand := range []string{src, dst} {
			if h := core.ColonLocalHint(cand); h != "" {
				fmt.Fprintln(stderr, h)
			}
		}
		if !core.IsRemote(src) {
			src = core.AbsPath(src)
		}
		if !core.IsRemote(dst) {
			dst = core.AbsPath(dst)
		}

		if !core.IsRemote(src) {
			if !isDirPath(src) {
				fmt.Fprintf(stderr, "✗ コピー元が存在しません: %s\n", src)
				lastRC = ExitError
				continue
			}
		}
		if !core.IsRemote(dst) {
			if fi, err := os.Stat(dst); err == nil && !fi.IsDir() {
				fmt.Fprintf(stderr, "✗ コピー先がフォルダではありません: %s\n", dst)
				lastRC = ExitError
				continue
			}
		}
		// コピー先がコピー元の内側だと、実行のたびにコピーが入れ子になって増え続ける。
		// 開始前に止める（リモート同士はパスの親子関係を判定できないので対象外）。
		if !core.IsRemote(src) && !core.IsRemote(dst) {
			if src == dst {
				fmt.Fprintf(stderr, "✗ コピー元とコピー先が同じです: %s\n", src)
				lastRC = ExitError
				continue
			}
			if core.IsInside(src, dst) {
				fmt.Fprintf(stderr, "✗ コピー先がコピー元の中にあります: %s\n", dst)
				fmt.Fprintln(stderr, "✗ コピー先には、コピー元の外（別フォルダや別ボリューム）を指定してください。")
				lastRC = ExitError
				continue
			}
		}

		fmt.Fprintf(stdout, "\n%s▶ %s → %s%s\n", c.bold, src, dst, c.reset)

		mountRoot := mountRootOf(opts)
		if !core.IsRemote(dst) && !core.DestIsMounted(dst, mountRoot) {
			fmt.Fprintf(stderr, "✗ コピー先のボリュームがマウントされていません: %s\n", dst)
			lastRC = ExitUnmounted
			continue
		}

		showProgress := outTTY
		if opts.NoProgress {
			showProgress = false
		}
		if opts.Progress {
			showProgress = true
		}

		job := &core.Job{
			Src: src, Dst: dst,
			Excludes:     opts.Excludes,
			FAT:          opts.FAT,
			Checksum:     opts.Checksum,
			IgnoreSpace:  opts.IgnoreSpaceCheck,
			Verbose:      opts.Stats,
			ShowProgress: showProgress,
			NoCaffeinate: opts.NoCaffeinate,
			DryRun:       opts.DryRun,
			Ctx:          ctx,
			Log:          core.LogMsg,
			Out:          stdout,
			Err:          stderr,
		}

		rc := core.RunJob(job, opts.VerifyOnly, opts.DryRun, !opts.NoVerify)
		if rc != 0 {
			lastRC = rc
		}
		if jobLoopStop(ctx, rc) {
			if lastRC == 0 {
				lastRC = ExitInterrupted
			}
			break
		}

		fmt.Fprintln(stdout)
		switch {
		case rc != 0:
			fmt.Fprintln(stderr, "✗ 一部の処理で問題が発生しました（上記のログを確認してください）")
		case opts.DryRun:
			fmt.Fprintln(stdout, "✓ dry-run を終えました（実際にはコピーしていません）")
		case opts.VerifyOnly:
			fmt.Fprintln(stdout, "✓ 照合のみ完了しました")
		case job.TotalFiles == 0:
			fmt.Fprintln(stdout, "✓ すべて完了（新しくコピーしたファイルはありません）")
		default:
			fmt.Fprintf(stdout, "%s✓ すべて完了（%d ファイル / %s / %s）%s\n",
				c.green, job.TotalFiles, core.FmtBytes(job.TotalBytes),
				core.FmtDuration(int64(time.Since(start).Seconds())), c.reset)
		}
	}
	return lastRC
}

// jobLoopStop は複数ジョブのループを止めるべきかを返す。
// シグナルで context がキャンセルされた、または直近のジョブが 130 を返した
// ときは、後続のジョブを実行しない。
func jobLoopStop(ctx context.Context, rc int) bool {
	return rc == ExitInterrupted || ctx.Err() != nil
}

// mountRootOf はコピー先判定に使うボリューム基点（環境変数 → 引数の順で決める）。
// 引数・環境変数ともに絶対パスへそろえる（bash 版と同じ）。
func mountRootOf(opts *core.Options) string {
	root := "/Volumes"
	if runtime.GOOS != "darwin" {
		root = "/"
	}
	if v := os.Getenv("SAFERSYNC_MOUNT_ROOT"); v != "" {
		root = v
	}
	if opts.MountRoot != "" {
		root = opts.MountRoot
	}
	return core.AbsPath(root)
}

// doStatus は接続状態の表示（bash 版 do_status と同じ書式）。
func doStatus(opts *core.Options, c colors, stdout io.Writer) int {
	mountRoot := mountRootOf(opts)
	fmt.Fprintf(stdout, "%sマウントされているボリューム%s（%s）\n", c.bold, c.reset, mountRoot)
	for _, v := range core.Volumes(mountRoot) {
		name := filepath.Base(v)
		fmt.Fprintf(stdout, "  %-20s ", name)
		if size, used, avail := core.DfHuman(v); size != "" {
			fmt.Fprintf(stdout, "%s 使用 / %s（空き %s）\n", used, size, avail)
		} else {
			fmt.Fprintln(stdout)
		}
	}
	fmt.Fprintln(stdout)
	return ExitOK
}

func isDirPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
