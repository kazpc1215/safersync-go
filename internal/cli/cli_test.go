package cli

import (
	"bytes"
	"testing"
)

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
	if err := validate(opts); err == nil || err.Error() != "コピー元を指定してください" {
		t.Errorf("want コピー元エラー, got %v", err)
	}
}

func TestRun_version(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"--version"}, &out, &errOut); code != ExitOK {
		t.Errorf("want exit 0, got %d", code)
	}
	if got := out.String(); got != "safersync v1.2.0\n" {
		t.Errorf("want %#v, got %#v", "safersync v1.2.0\n", got)
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
