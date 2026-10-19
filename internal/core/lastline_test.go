package core

// lastline_test.go - CurrentFile が使う lastLine（末尾から遡る読み取り）の回帰テスト。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastLine(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{"存在しないファイル", filepath.Join(dir, "nope"), ""},
		{"空ファイル", write("empty", ""), ""},
		{"改行なし", write("one", "a.txt"), "a.txt"},
		{"末尾に改行", write("nl", "a.txt\n"), "a.txt"},
		{"複数行", write("multi", "a\nb\nc\n"), "c"},
		{"末尾に空行", write("blank", "a\nlast\n\n\n"), "last"},
		{"空行のみ", write("onlyblank", "\n\n\n"), ""},
		{"前後の空白を除去", write("ws", "a\n\t b.txt \n"), "b.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastLine(tc.path); got != tc.want {
				t.Errorf("lastLine = %q, want %q", got, tc.want)
			}
		})
	}
}

// チャンク境界をまたぐ大きなログでも末尾の行を正しく返す。
func TestLastLineLargeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.log")
	var b strings.Builder
	for i := 0; i < 20000; i++ { // 約 300 KiB（1 チャンク 64 KiB を超える）
		b.WriteString("ファイルを送信中\n")
	}
	b.WriteString("最終ファイル.txt\n")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lastLine(p); got != "最終ファイル.txt" {
		t.Errorf("lastLine = %q, want %q", got, "最終ファイル.txt")
	}
}

// 1 行がチャンクより長くても（先頭が途中で切れても）正しく返す。
func TestLastLineLongSingleLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "long.log")
	long := strings.Repeat("x", 200<<10) // 200 KiB、改行なし
	if err := os.WriteFile(p, []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lastLine(p); got != long {
		t.Errorf("長い1行を取得できない: got %d バイト, want %d バイト", len(got), len(long))
	}
}
