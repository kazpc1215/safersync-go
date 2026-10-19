package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchExclude_basename(t *testing.T) {
	// "/" も "**" も含まないパターンは最終要素に対して一致する
	cases := []struct {
		pat, rel string
		isDir    bool
		want     bool
	}{
		{"*.tmp", "a/b/c.tmp", false, true},
		{"*.tmp", "a/b/c.txt", false, false},
		{"cache", "x/cache", true, true},
		{"cache", "x/cache", false, true}, // "/" 無しはファイル・ディレクトリ両方に一致
		{"cache/", "x/cache", true, true},
		{"cache/", "x/cache", false, false}, // 末尾 "/" はディレクトリ限定
		{".DS_Store", "a/.DS_Store", false, true},
		{"**.tmp", "a/b/c.tmp", false, true}, // "**" を含むのでパス照合だが末尾一致で拾える
	}
	for _, c := range cases {
		if got := MatchExclude(c.pat, c.rel, c.isDir); got != c.want {
			t.Errorf("MatchExclude(%q, %q, dir=%v) = %v, want %v", c.pat, c.rel, c.isDir, got, c.want)
		}
	}
}

func TestMatchExclude_pathPattern(t *testing.T) {
	// "/" を含むパターンは転送ルート相対のパスに一致（任意の境界から）
	cases := []struct {
		pat, rel string
		want     bool
	}{
		{"sub/cache", "sub/cache", true},
		{"sub/cache", "a/sub/cache", true},
		{"a/sub/cache", "a/sub/cache", true},
		{"a/sub/cache", "b/a/sub/cache", true}, // 非固定は任意の境界から
		{"sub/cache", "sub/other", false},
		{"sub/*", "sub/cache", true},
		{"sub/*", "sub/x/cache", false}, // "*" は "/" をまたがない
		{"sub/**", "sub/x/cache", true},
		{"/top", "top", true},    // 先頭 "/" はルート固定
		{"/top", "a/top", false}, //         ルート以外は一致しない
		{"/a/b", "a/b", true},
		{"/a/b", "x/a/b", false},
	}
	for _, c := range cases {
		if got := MatchExclude(c.pat, c.rel, false); got != c.want {
			t.Errorf("MatchExclude(%q, %q) = %v, want %v", c.pat, c.rel, got, c.want)
		}
	}
}

func TestMatchExclude_charClassAndQuestion(t *testing.T) {
	cases := []struct {
		pat, rel string
		want     bool
	}{
		{"file?.txt", "file1.txt", true},
		{"file?.txt", "file12.txt", false},
		{"file?.txt", "file/.txt", false}, // "?" は "/" に一致しない
		{"[ab]c", "ac", true},
		{"[ab]c", "bc", true},
		{"[ab]c", "cc", false},
		{"[a-c]z", "bz", true},
		{"[!a]z", "bz", true},
		{"[!a]z", "az", false},
	}
	for _, c := range cases {
		if got := MatchExclude(c.pat, c.rel, false); got != c.want {
			t.Errorf("MatchExclude(%q, %q) = %v, want %v", c.pat, c.rel, got, c.want)
		}
	}
}

// ScanDir がパス依存の除外（"/" や末尾 "/"）を rsync と同じように枝ごと落とす
func TestScanDirPathPatterns(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel, data string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("keep/a.txt", "aaa")        // 常に残す
	mk("sub/cache/b.txt", "bbb")   // ディレクトリ cache
	mk("other/cache/c.txt", "ccc") // 別階層のディレクトリ cache
	mk("y/cache", "ddd")           // ファイル名 cache（ディレクトリではない）
	mk("DataCache/d.bin", "eee")

	// パス指定の除外: "sub/cache" はその枝だけ落とす（other/cache は残る）
	if got := ScanDir(dir, []string{"sub/cache"}).Files; got != 4 {
		t.Errorf(`"sub/cache" 除外後 = %d, want 4`, got)
	}
	// "cache/" はディレクトリだけに一致（ファイル y/cache は残る）
	if got := ScanDir(dir, []string{"cache/"}).Files; got != 3 {
		t.Errorf(`"cache/" 除外後 = %d, want 3（ファイル名 cache は残る）`, got)
	}
	// "cache"（末尾スラッシュ無し）はファイル名にも一致
	if got := ScanDir(dir, []string{"cache"}).Files; got != 2 {
		t.Errorf(`"cache" 除外後 = %d, want 2（同名ファイルも除外）`, got)
	}
}

// "**/" は 0 個以上のディレクトリに一致する（ルート直下も対象）。
func TestMatchExclude_doubleStarSlash(t *testing.T) {
	cases := []struct {
		pattern, path string
		isDir         bool
		want          bool
	}{
		{"**/*.tmp", "x.tmp", false, true},
		{"**/*.tmp", "a/x.tmp", false, true},
		{"**/*.tmp", "a/b/x.tmp", false, true},
		{"**/*.tmp", "x.txt", false, false},
		{"**/cache", "cache", true, true},
		{"**/cache", "a/cache", true, true},
		{"**/cache/", "cache", true, true},
		{"**/cache/", "cache", false, false},
		{"a/**/b", "a/b", false, true},
		{"a/**/b", "a/x/b", false, true},
		{"a/**/b", "a/x/y/b", false, true},
		{"a/**/b", "a/x/c", false, false},
	}
	for _, c := range cases {
		if got := MatchExclude(c.pattern, c.path, c.isDir); got != c.want {
			t.Errorf("MatchExclude(%q, %q, %v) = %v, want %v", c.pattern, c.path, c.isDir, got, c.want)
		}
	}
}
