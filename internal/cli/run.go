package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kazpc1215/safersync/internal/core"
	"github.com/kazpc1215/safersync/internal/version"
)

// colors は端末のときだけ使う ANSI 色。非端末では全て空にする（bash 版と同じ）。
type colors struct{ bold, red, green, yellow, blue, reset string }

func newColors(stdout bool) colors {
	if !stdout {
		return colors{}
	}
	return colors{bold: "\x1b[1m", red: "\x1b[31m", green: "\x1b[32m",
		yellow: "\x1b[33m", blue: "\x1b[34m", reset: "\x1b[0m"}
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Run は safersync の本体。bash 版 main() と同じ流れ・出力・終了コード。
func Run(argv []string, stdout, stderr io.Writer) int {
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
	c := newColors(isTTY(os.Stdout))

	if opts.ShowStatus {
		return doStatus(opts, c, stdout)
	}

	// --- 対話ウィザード（TUI）---
	//   引数なしで対話端末なら自動起動。--tui/-i なら明示的に起動する。
	inTTY, outTTY := isTTY(os.Stdin), isTTY(os.Stdout)
	if opts.TUI || (noArgs && inTTY && outTTY) {
		if !inTTY || !outTTY {
			fmt.Fprintln(stderr, "✗ TUI は対話端末でのみ起動できます（--help を参照）")
			return ExitError
		}
		// ステップ 3（Bubble Tea）で実装する
		return notImplemented(stderr, "対話ウィザード（TUI）")
	}

	// --- コピー元とコピー先 ---
	if opts.From == "" {
		fmt.Fprintln(stderr, "✗ コピー元を指定してください（例: safersync ~/書類 /Volumes/HDD/書類）")
		return ExitError
	}
	if opts.To == "" {
		fmt.Fprintln(stderr, "✗ コピー先を指定してください（例: safersync ~/書類 /Volumes/HDD/書類）")
		return ExitError
	}

	src := core.AbsPath(opts.From)
	dst := core.AbsPath(opts.To)

	if !isDirPath(src) {
		fmt.Fprintf(stderr, "✗ コピー元が存在しません: %s\n", src)
		return ExitError
	}
	if fi, err := os.Stat(dst); err == nil && !fi.IsDir() {
		fmt.Fprintf(stderr, "✗ コピー先がフォルダではありません: %s\n", dst)
		return ExitError
	}

	fmt.Fprintf(stdout, "\n%s▶ %s → %s%s\n", c.bold, src, dst, c.reset)

	// 未マウントの外付けボリュームへ書き込まない
	mountRoot := mountRootOf(opts)
	if !core.DestIsMounted(dst, mountRoot) {
		fmt.Fprintf(stderr, "✗ コピー先のボリュームがマウントされていません: %s\n", dst)
		fmt.Fprintln(stderr, "✗ 外付け媒体を接続してから再実行してください。")
		fmt.Fprintf(stderr, "✗ 現在の %s: %s\n", mountRoot, strings.Join(volumeNames(mountRoot), " "))
		return ExitUnmounted
	}

	// 進捗表示（既定はターミナルのときだけ ON。パイプやログでは出さない）
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
		Log:          core.LogMsg,
		Out:          stdout,
		Err:          stderr,
	}

	rc := core.RunJob(job, opts.VerifyOnly, opts.DryRun, !opts.NoVerify)

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
	return rc
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

func volumeNames(mountRoot string) []string {
	vols := core.Volumes(mountRoot)
	names := make([]string, 0, len(vols))
	for _, v := range vols {
		names = append(names, filepath.Base(v))
	}
	return names
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

// notImplemented は未実装の機能に一時的な案内を出す。
func notImplemented(stderr io.Writer, what string) int {
	fmt.Fprintf(stderr, "✗ この機能はまだ移植中です: %s\n（bash 版 ~/safersync/bin/safersync をご利用ください）\n", what)
	return ExitError
}
