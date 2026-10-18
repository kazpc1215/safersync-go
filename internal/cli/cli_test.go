package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kazpc1215/safersync/internal/core"
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
	if err := validateFromTo(opts); err == nil || err.Error() != "コピー元を指定してください（例: safersync ~/書類 /Volumes/HDD/書類）" {
		t.Errorf("want コピー元エラー, got %v", err)
	}
}

// validateFromTo は run.go で使う引用チェックのテスト用に再現する
func validateFromTo(opts *core.Options) error {
	if opts.From == "" {
		return errors.New("コピー元を指定してください（例: safersync ~/書類 /Volumes/HDD/書類）")
	}
	if opts.To == "" {
		return errors.New("コピー先を指定してください（例: safersync ~/書類 /Volumes/HDD/書類）")
	}
	return nil
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
	if got := out.String(); got != "safersync v1.3.0\n" {
		t.Errorf("want %#v, got %#v", "safersync v1.3.0\n", got)
	}
}

func TestRun_unimplementedCopy(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"/src", "/dst"}, &out, &errOut); code != ExitError {
		t.Errorf("want exit 1, got %d", code)
	}
	if got := errOut.String(); got == "" {
		t.Error("未実装メッセージが出ていない")
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
