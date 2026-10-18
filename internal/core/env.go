// Package core は画面出力を持たない純ロジック（環境判定・disk・sync・verify）。
// bash 版 lib/core/ の Go 移植。
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// ファイルサイズ・時間の整形（bash 版 env.sh の fmt_bytes / fmt_duration と同じ）
// ---------------------------------------------------------------------------

// FmtBytes はバイト数を「12.3 GB」のように 1024 基準で 1 桁小数にする。
func FmtBytes[T int | int64 | uint64](b T) string {
	v := float64(b)
	unit := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= 1024 && i < len(unit)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, unit[i])
}

// FmtDuration は秒数を「1時間2分」「2分3秒」「4秒」の形にする。
func FmtDuration(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%d時間%d分", h, m)
	case m > 0:
		return fmt.Sprintf("%d分%d秒", m, s)
	default:
		return fmt.Sprintf("%d秒", s)
	}
}

// ---------------------------------------------------------------------------
// パス helpers（bash 版 env.sh の strip_trailing_slashes / abspath / expand_tilde）
// ---------------------------------------------------------------------------

// StripTrailingSlashes は末尾の "/" を取り除く（"/" そのものは残す）。
// "SRC/" と "SRC" を同じ意味にそろえ、書き方の違いによる事故を防ぐ。
func StripTrailingSlashes(p string) string {
	for p != "/" && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// ExpandTilde は先頭の "~" をホームディレクトリに置き換える。
func ExpandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// AbsPath はパスをカレントディレクトリ基準の絶対パスにして末尾 "/" を取り除く。
// 実行時のどこにいても同じ意味になる（bash 版 env.sh 同様、存在しなくてもよい）。
func AbsPath(p string) string {
	p = ExpandTilde(p)
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd(), p)
	}
	return StripTrailingSlashes(p)
}

func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "/"
	}
	return dir
}

// ---------------------------------------------------------------------------
// ログ（bash 版 env.sh の LOG_DIR / LOG_FILE / log_msg）
// ---------------------------------------------------------------------------

// LogDir はログの出力先ディレクトリを決める。bash 版 env.sh と同じ優先順位。
//  1. $SAFERSYNC_LOG_DIR
//  2. $XDG_STATE_HOME/safersync（macOS でも優先）
//  3. macOS: ~/Library/Logs/safersync
//  4. それ以外: ~/.local/share/safersync
func LogDir() string {
	if v := os.Getenv("SAFERSYNC_LOG_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "safersync")
	}
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "safersync")
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "safersync")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "safersync")
}

// LogFile はログファイルのフルパス。
func LogFile() string { return filepath.Join(LogDir(), "safersync.log") }

// LogMsg はログに1行追記する（bash 版 log_msg。失敗しても黙って無視する）。
func LogMsg(msg string) {
	_ = os.MkdirAll(LogDir(), 0o755)
	f, err := os.OpenFile(LogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}
