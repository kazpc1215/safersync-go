package main

import (
	"bytes"
	"strings"
	"testing"
)

// main パッケージのエントリ（run）が終了コードと出力を返すことを確認する。
// 実処理は internal/cli 側でテスト済みなので、ここは配線の確認にとどめる。
func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := run([]string{"--version"}, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d, stderr = %s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "safersync v") {
		t.Errorf("バージョン出力が無い: %q", out.String())
	}
}

func TestRunUnknownOption(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := run([]string{"--no-such-option"}, &out, &errb); rc != 1 {
		t.Errorf("rc = %d, want 1", rc)
	}
	if !strings.Contains(errb.String(), "不明なオプション") {
		t.Errorf("エラー出力が無い: %q", errb.String())
	}
}
