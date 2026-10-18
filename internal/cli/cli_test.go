package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kazpc1215/safersync/internal/core"
	"github.com/kazpc1215/safersync/internal/version"
)

func TestIsTTY_devNullIsNotTTY(t *testing.T) {
	// /dev/null はキャラクタデバイスだが端末ではない。
	// 以前はここを端末と誤認し、「TUI は対話端末でのみ」チェックをすり抜けていた。
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTTY(f) {
		t.Error("/dev/null を端末と誤認している")
	}

	// 通常のファイルも端末ではない
	tmp, err := os.CreateTemp(t.TempDir(), "f")
	if err != nil {
		t.Fatal(err)
	}
	defer tmp.Close()
	if isTTY(tmp) {
		t.Error("通常ファイルを端末と誤認している")
	}
}

func TestParse_versionFlag(t *testing.T) {
	opts, err := Parse([]string{"--version"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if !opts.ShowVersion {
		t.Error("ShowVersion が true になっていない")
	}
}

func TestParse_positional(t *testing.T) {
	opts, err := Parse([]string{"/src", "/dst"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if opts.From != "/src" || opts.To != "/dst" {
		t.Errorf("From=%q To=%q", opts.From, opts.To)
	}
}

func TestParse_fromToOrder(t *testing.T) {
	// --to を先に書いても順序は自由
	opts, err := Parse([]string{"--to", "/dst", "/src"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if opts.From != "/src" || opts.To != "/dst" {
		t.Errorf("From=%q To=%q", opts.From, opts.To)
	}
}

func TestParse_progressLastWins(t *testing.T) {
	// bash 版と同じく、--progress / --no-progress は最後の指定が勝つ
	opts, err := Parse([]string{"--progress", "--no-progress", "/src", "/dst"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if opts.Progress || !opts.NoProgress {
		t.Errorf("--progress --no-progress → Progress=%v NoProgress=%v（NoProgress が勝つべき）",
			opts.Progress, opts.NoProgress)
	}

	opts, err = Parse([]string{"--no-progress", "--progress", "/src", "/dst"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if !opts.Progress || opts.NoProgress {
		t.Errorf("--no-progress --progress → Progress=%v NoProgress=%v（Progress が勝つべき）",
			opts.Progress, opts.NoProgress)
	}
}

func TestParse_tooManyArgs(t *testing.T) {
	_, err := Parse([]string{"x", "y", "z"})
	if err == nil {
		t.Fatal("エラーになるべき")
	}
	want := "引数が多すぎます: 'z'（コピー元とコピー先を1つずつ指定してください）"
	if want != err.Error() {
		t.Errorf("want %q, got %q", want, err.Error())
	}
}

func TestParse_missingFrom(t *testing.T) {
	opts, err := Parse([]string{"--to", "/dst"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if opts.From != "" {
		t.Fatalf("From が空のはず")
	}
}

// コピー元・コピー先が欠けているときは、本番の Run が案内を出して終了コード 1 で
// 止まること（テスト専用の再実装ではなく実装そのものを検証する）。
func TestRun_missingFromTo(t *testing.T) {
	if !core.RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"--to", "/dst"}, &out, &errOut); code != ExitError {
		t.Fatalf("want exit %d, got %d (stderr=%q)", ExitError, code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "コピー元とコピー先を指定してください") {
		t.Errorf("案内メッセージが出ていない: %q", errOut.String())
	}
	if strings.Contains(out.String(), "▶") {
		t.Errorf("引数不足なのに処理を始めている: %q", out.String())
	}
}

func TestParse_duplicateFrom(t *testing.T) {
	// bash 版と同じ: --from が確保済みなら位置引数はコピー先に入る。重複はエラー。
	opts, err := Parse([]string{"--from", "/a", "/b"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if opts.From != "/a" || opts.To != "/b" {
		t.Errorf("want /a→/b, got From=%q To=%q", opts.From, opts.To)
	}
	if _, err := Parse([]string{"--from", "/a", "--from", "/b"}); err == nil {
		t.Fatal("コピー元が重複したらエラーになるべき")
	}
}

func TestParse_unknownFlag(t *testing.T) {
	_, err := Parse([]string{"--bogus"})
	if err == nil {
		t.Fatal("不明なフラグはエラーになるべき")
	}
}

func TestParse_afterDoubleDash(t *testing.T) {
	opts, err := Parse([]string{"-n", "--", "x", "y z"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if !opts.DryRun || opts.From != "x" || opts.To != "y z" {
		t.Errorf("opts=%+v", opts)
	}
}

func TestCoreFmtBytes(t *testing.T) {
	if got := core.FmtBytes(int64(13207170048)); got != "12.3 GB" {
		t.Errorf("bad fmtbytes: %q", got)
	}
	if got := core.FmtDuration(130); got != "2分10秒" {
		t.Errorf("bad duration: %q", got)
	}
}

func TestCoreStripTrailingSlashes(t *testing.T) {
	cases := map[string]string{
		"/":       "/",
		"/a/":     "/a",
		"/a//":    "/a",
		"rel/dir": "rel/dir",
	}
	for in, want := range cases {
		if got := core.StripTrailingSlashes(in); got != want {
			t.Errorf("StripTrailingSlashes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRun_version(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"--version"}, &out, &errOut); code != ExitOK {
		t.Errorf("want exit 0, got %d", code)
	}
	want := "safersync v" + version.Version + "\n"
	if got := out.String(); got != want {
		t.Errorf("want %#v, got %#v", want, got)
	}
}

// コピー元フォルダが存在しないときは、コピーを始めずに終了コード 1 で止まること。
func TestRun_missingSourceDir(t *testing.T) {
	if !core.RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"/nonexistent-src", "/dst"}, &out, &errOut); code != ExitError {
		t.Errorf("want exit %d, got %d (stderr=%q)", ExitError, code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "コピー元が存在しません") {
		t.Errorf("コピー元が無い旨のメッセージが出ていない: %q", errOut.String())
	}
}

// コピー先がコピー元の中にあるときは、入れ子コピーを始めずに止まること。
func TestRun_destInsideSource(t *testing.T) {
	if !core.RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src := t.TempDir()
	dst := filepath.Join(src, "backup")

	var out, errOut bytes.Buffer
	if code := Run([]string{src, dst}, &out, &errOut); code != ExitError {
		t.Fatalf("want exit %d, got %d (stderr=%q)", ExitError, code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "コピー先がコピー元の中にあります") {
		t.Errorf("案内メッセージが出ていない: %q", errOut.String())
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("指定が拒否されたのにコピー先が作られている: %v", err)
	}
}

// コピー元とコピー先が同じときも止まること。
func TestRun_sameSourceAndDest(t *testing.T) {
	if !core.RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src := t.TempDir()

	var out, errOut bytes.Buffer
	if code := Run([]string{src, src}, &out, &errOut); code != ExitError {
		t.Fatalf("want exit %d, got %d (stderr=%q)", ExitError, code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "コピー元とコピー先が同じです") {
		t.Errorf("案内メッセージが出ていない: %q", errOut.String())
	}
}

func TestParse_jobRequiresTwoPaths(t *testing.T) {
	if _, err := Parse([]string{"--job", "/src"}); err == nil {
		t.Fatal("--job の引数不足はエラーになるべき")
	}
	if _, err := Parse([]string{"--job", "/src", ""}); err == nil {
		t.Fatal("--job に空のパスはエラーになるべき")
	}
	opts, err := Parse([]string{"--job", "/src", "/dst", "--job", "/src2", "/dst2"})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if len(opts.Jobs) != 2 || opts.Jobs[1].From != "/src2" || opts.Jobs[1].To != "/dst2" {
		t.Fatalf("--job が2組とも入るはず: %+v", opts.Jobs)
	}
}

func TestParse_jobConflictsWithSinglePair(t *testing.T) {
	// どちらかが黙って無視されないよう、併用はエラーにする
	if _, err := Parse([]string{"--job", "/src", "/dst", "/extra"}); err == nil {
		t.Fatal("--job と位置引数の併用はエラーになるべき")
	}
	if _, err := Parse([]string{"--from", "/a", "--job", "/src", "/dst"}); err == nil {
		t.Fatal("--job と --from の併用はエラーになるべき")
	}
}

// rsync が無い環境では、成功と誤報告せずに環境エラー（3）で止まること
func TestRun_rsyncMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	src, dst := t.TempDir(), t.TempDir()
	var out, errOut bytes.Buffer
	code := Run([]string{src, dst}, &out, &errOut)
	if code != ExitEnv {
		t.Fatalf("want exit %d, got %d (stdout=%q stderr=%q)", ExitEnv, code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "rsync") {
		t.Errorf("rsync が見つからない旨のメッセージが出ていない: %q", errOut.String())
	}
	if strings.Contains(out.String(), "すべて最新") {
		t.Errorf("差分を確認できていないのに成功と誤報告している: %q", out.String())
	}
}

// フラグの一括マッピング（長短・= 形式・--exclude の複数指定）
func TestParse_allFlags(t *testing.T) {
	opts, err := Parse([]string{
		"--tui", "--dry-run", "--checksum", "--fat", "--ignore-space-check",
		"-v", "--no-caffeinate", "--no-verify",
		"--exclude", "*.tmp", "--exclude=secret", "-x", "raw",
		"--mount-root=/mnt/vols",
		"--from", "/src", "--to=/dst",
	})
	if err != nil {
		t.Fatalf("エラー: %v", err)
	}
	if !opts.TUI || !opts.DryRun || !opts.Checksum || !opts.FAT || opts.APFS ||
		!opts.IgnoreSpaceCheck || !opts.Stats || !opts.NoCaffeinate || !opts.NoVerify {
		t.Errorf("フラグが Options に反映されていない: %+v", opts)
	}
	if strings.Join(opts.Excludes, ",") != "*.tmp,secret,raw" {
		t.Errorf("Excludes = %v", opts.Excludes)
	}
	if opts.MountRoot != "/mnt/vols" || opts.From != "/src" || opts.To != "/dst" {
		t.Errorf("MountRoot/From/To = %q / %q / %q", opts.MountRoot, opts.From, opts.To)
	}
}

// --fat と --apfs は後勝ち
func TestParse_fatAPFSLastWins(t *testing.T) {
	opts, err := Parse([]string{"--fat", "--apfs", "/a", "/b"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.FAT || !opts.APFS {
		t.Errorf("--apfs が最後: FAT=%v APFS=%v", opts.FAT, opts.APFS)
	}
	opts, err = Parse([]string{"--apfs", "--fat", "/a", "/b"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.FAT || opts.APFS {
		t.Errorf("--fat が最後: FAT=%v APFS=%v", opts.FAT, opts.APFS)
	}
}

func TestParse_helpStatusTUI(t *testing.T) {
	if opts, err := Parse([]string{"--help"}); err != nil || !opts.ShowHelp {
		t.Errorf("--help: err=%v opts=%+v", err, opts)
	}
	if opts, err := Parse([]string{"--status"}); err != nil || !opts.ShowStatus {
		t.Errorf("--status: err=%v opts=%+v", err, opts)
	}
	if opts, err := Parse([]string{"-i"}); err != nil || !opts.TUI {
		t.Errorf("-i: err=%v opts=%+v", err, opts)
	}
}

// mountRootOf: 引数 > 環境変数 > プラットフォーム既定 の優先順
func TestMountRootOf(t *testing.T) {
	opts := &core.Options{}
	t.Setenv("SAFERSYNC_MOUNT_ROOT", "")
	want := "/Volumes"
	if runtime.GOOS != "darwin" {
		want = "/"
	}
	if got := mountRootOf(opts); got != want {
		t.Errorf("既定 = %q, want %q", got, want)
	}
	t.Setenv("SAFERSYNC_MOUNT_ROOT", "/mnt/vols/")
	if got := mountRootOf(opts); got != "/mnt/vols" {
		t.Errorf("環境変数 = %q, want /mnt/vols（末尾スラッシュは落とす）", got)
	}
	opts.MountRoot = "/custom/dir/"
	if got := mountRootOf(opts); got != "/custom/dir" {
		t.Errorf("引数優先 = %q, want /custom/dir", got)
	}
}

// doStatus は基点直下のフォルダを一覧表示する
func TestDoStatus(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"diskA", "diskB"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	rc := doStatus(&core.Options{MountRoot: root}, colors{}, &buf)
	if rc != ExitOK {
		t.Fatalf("rc = %d, want %d", rc, ExitOK)
	}
	out := buf.String()
	for _, want := range []string{"マウントされているボリューム", root, "diskA", "diskB"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q がない:\n%s", want, out)
		}
	}
}

// 値が必要なオプションに値が無いときは、黙殺せずエラーにすること。
// （--from / --to はエラー、--exclude / --mount-root は無視、という不揃いを解消）
func TestParse_missingOptionValue(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"--exclude 値なし", []string{"--exclude"}, "--exclude"},
		{"-x 値なし", []string{"-x"}, "--exclude"},
		{"--exclude= が空", []string{"--exclude="}, "--exclude"},
		{"--mount-root 値なし", []string{"--mount-root"}, "--mount-root"},
		{"--mount-root= が空", []string{"--mount-root="}, "--mount-root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.argv)
			if err == nil {
				t.Fatalf("エラーになるはず: %v", c.argv)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("エラーに %q が含まれない: %v", c.want, err)
			}
		})
	}
	// 値があれば従来どおり通ること
	if _, err := Parse([]string{"--exclude", "*.tmp", "--mount-root", "/mnt"}); err != nil {
		t.Errorf("値ありは通るはず: %v", err)
	}
}

// --help の終了コード表は bash 版の BATS（部分文字列 "2 コピー先が未マウント" /
// "3 空き容量不足"）と契約を共有する。文言を変えると気づけるように固定しておく。
func TestHelp_exitCodeContract(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "/tmp/safersync.log")
	out := buf.String()
	for _, want := range []string{
		"終了コード",
		"1 エラー",
		"2 コピー先が未マウント",
		"3 空き容量不足",
		"130 中断",
		"--verify-only",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--help に %q が無い", want)
		}
	}
	// --verify-only は --dry-run との優先関係が分かるようにしておく
	if !strings.Contains(out, "--dry-run より優先") {
		t.Errorf("--help に --verify-only の優先関係の説明が無い")
	}
}
