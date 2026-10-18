package core

// disk.go - ボリューム検知・空き容量・マウント判定・走査
// bash 版 lib/core/disk.sh の Go 移植。
// os.Stat / syscall.Statfstatfs を直呼びするため、bash 版にあった
// `stat -f` / `stat -c` の OS 差は存在しない。

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// ---------------------------------------------------------------------------
// マウント判定
// ---------------------------------------------------------------------------

// IsMountPoint はパスそのものがマウントポイントかどうか。
// bash 版と同じく `mount` の出力に " on <path> " が現れるかで判定する。
func IsMountPoint(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return false
	}
	out, err := exec.Command("mount").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), " on "+path+" ")
}

// Dirname は親ディレクトリへ1段上がる（"x" は "." ではなく親を返す点に注意）。
func Dirname(p string) string {
	if p == "/" {
		return "/"
	}
	return filepath.Dir(p)
}

// MountPointOf はパス（存在しなければ最も近い既存の祖先）から上へたどり、
// 最初に見つかる実マウントポイントを返す。見つからなければ "/"。
func MountPointOf(p string) string {
	for !exists(p) && p != "/" {
		p = Dirname(p)
	}
	for {
		if IsMountPoint(p) {
			return p
		}
		if p == "/" {
			return "/"
		}
		p = Dirname(p)
	}
}

func exists(p string) bool {
	_, err := os.Stat(p) // リンク切れなどは存在しない扱い（bash の -e と同等）
	return err == nil
}

// DestIsMounted は「コピー先がマウント基点の下にあるのに、そのボリュームが実
// マウントされていなければ false」を返す。基点の外（~/ や /tmp）は判定しない。
func DestIsMounted(dst, mountRoot string) bool {
	if mountRoot == "/" {
		return true
	}
	if !strings.HasPrefix(dst, mountRoot+"/") {
		return true // 基点の下ではない → 対象外
	}
	mp := MountPointOf(dst)
	return strings.HasPrefix(mp, mountRoot+"/")
}

// ---------------------------------------------------------------------------
// 空き容量
// ---------------------------------------------------------------------------

// FreeBytes は指定パス（存在しなければ最も近い既存の祖先）の空き容量をバイトで返す。
// bash 版 free_bytes（df -k の使用可能ブロック × 1024）と同じ値になる。
// 取得できない場合は 0。
func FreeBytes(p string) int64 {
	for !exists(p) && p != "/" {
		p = Dirname(p)
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

// TotalBytes は指定パスの総容量をバイトで返す（FreeBytes と対で使う）。
func TotalBytes(p string) int64 {
	for !exists(p) && p != "/" {
		p = Dirname(p)
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0
	}
	return int64(st.Blocks) * int64(st.Bsize)
}

// ---------------------------------------------------------------------------
// 走査（ファイル数・合計バイト数）
// ---------------------------------------------------------------------------

// ScanResult は exclude を適用した走査の結果。
type ScanResult struct {
	Files int
	Bytes int64
}

// ScanDir は dir を走査し、除外パターン（ファイル名の glob）に一致する場所を
// まるごと枝ごと落として（bash 版 find -prune と同じ）、
// ファイル数と合計バイト数を返す。走査中のエラー（権限など）は無視する。
func ScanDir(dir string, excludes []string) ScanResult {
	var res ScanResult
	// 既定の除外（DEFAULT_EXCLUDES 相当）+ ユーザー指定を合わせて適用する。
	// bash 版は rsync の --exclude と同じ組み合わせを find -prune にも使う。
	pats := append(append([]string{}, DefaultExcludes...), excludes...)
	excl := make([]string, 0, len(pats))
	for _, e := range pats {
		if e != "" {
			excl = append(excl, e)
		}
	}

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == dir {
			return nil
		}
		name := d.Name()
		for _, pat := range excl {
			if ok, _ := filepath.Match(pat, name); ok {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				res.Files++
				res.Bytes += info.Size()
			}
		}
		return nil
	})
	return res
}

// Volumes はマウント基点直下にあるフォルダの一覧を返す（--status 用）。
// bash 版と同じくシンボリックリンクの先も評価する（/Volumes/Macintosh HD → / など）。
// 読めなければ空。
func Volumes(mountRoot string) []string {
	entries, err := os.ReadDir(mountRoot)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		fi, err := os.Stat(filepath.Join(mountRoot, e.Name())) // リンク先を追跡
		if err != nil || !fi.IsDir() {
			continue
		}
		out = append(out, filepath.Join(mountRoot, e.Name()))
	}
	return out
}

// DfHuman は df -h と同等の情報を得る（--status の表示用）。取得できなければ nil。
func DfHuman(path string) (size, used, avail string) {
	out, err := exec.Command("df", "-h", path).Output()
	if err != nil {
		return "", "", ""
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 2 {
		return "", "", ""
	}
	f := strings.Fields(lines[1])
	if len(f) < 4 {
		return "", "", ""
	}
	return f[1], f[2], f[3]
}
