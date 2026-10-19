// Package core は画面出力を持たない純ロジック（環境判定・disk・sync・verify）。
// bash 版 lib/core/ の Go 移植。
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// IsRemote は rsync のリモートパス（host:path / user@host:path / rsync://）かどうかを返す。
//
// 判定は rsync と同じで、最初のスラッシュ（またはバックスラッシュ）より前に ":" が
// あればリモート扱いにする。したがってローカルのファイル名に ":" が含まれる場合は、
// "./name:tag" のように先頭へ "./" を付けるとローカルとして扱える（rsync と同じ回避策）。
func IsRemote(p string) bool {
	if strings.HasPrefix(p, "rsync://") {
		return true
	}
	idx := strings.IndexAny(p, "/\\")
	if idx < 0 {
		idx = len(p)
	}
	return strings.Contains(p[:idx], ":")
}

// ColonLocalHint は、ローカルに存在するのに ":" のせいで rsync にリモート扱い
// されるパスへ、修正方法（先頭に "./" を付ける）を促す一文を返す。該当しなければ ""。
//
// rsync は最初の "/" より前の ":" を「ホスト名の区切り」とみなすため、ローカルの
// 「2019:写真」のようなフォルダを渡すと「ホスト名を解決できません」という分かり
// にくい失敗になる。リテラルのパスが実在するときだけヒントを出す（本物のリモートは
// ローカルに実在しないため誤爆しない）。
func ColonLocalHint(p string) string {
	if p == "" || !IsRemote(p) {
		return ""
	}
	if _, err := os.Lstat(p); err != nil {
		return ""
	}
	return fmt.Sprintf(
		"ヒント: %q はローカルにありますが、最初の \":\" より前がホスト名と解釈されリモート扱いになります。ローカルのつもりなら %q のように指定してください。",
		p, "./"+p)
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

// IsInside は child が parent と同じか、parent の中にあるかを返す。
//
// 「コピー先がコピー元の中にある」状態で実行すると、追加のみ（--delete なし）の
// 設計のため、実行のたびにコピーが入れ子（backup/backup/...）になり容量を
// 圧迫し続ける。開始前にこの事故を検出するために使う。
// 相対パスはカレント基準の絶対パスにしてから比較し、".." も解決する。
func IsInside(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	parent = filepath.Clean(AbsPath(parent))
	child = filepath.Clean(AbsPath(child))
	if parent == child {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
//  4. それ以外: $XDG_DATA_HOME/safersync（無ければ ~/.local/share/safersync）
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
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "safersync")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "safersync")
}

// LogFile はログファイルのフルパス。
func LogFile() string { return filepath.Join(LogDir(), "safersync.log") }

// logMaxBytes を超えたら1世代だけローテーションする（safersync.log.1）。
// 長時間の運用でログが無限に育つのを防ぐ。
const logMaxBytes = 5 << 20 // 5 MiB

// logMu は LogMsg の追記・ローテーションを直列化する（TUI のゴルーチンからも呼ばれる）。
var logMu sync.Mutex

// LogMsg はログに1行追記する（bash 版 log_msg。失敗しても黙って無視する）。
func LogMsg(msg string) {
	logMu.Lock()
	defer logMu.Unlock()

	_ = os.MkdirAll(LogDir(), 0o755)
	path := LogFile()
	if fi, err := os.Stat(path); err == nil && fi.Size() > logMaxBytes {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}
