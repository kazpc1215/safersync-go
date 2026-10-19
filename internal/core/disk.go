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
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// mount 出力のキャッシュ
// ---------------------------------------------------------------------------

// mount コマンドは1回の exec（プロセス起動）を伴うため、マウント判定のたびに
// 実行すると、祖先を1段ずつたどる mountPointOf や複数ボリュームの列挙で
// 積み上がる。TTL 付きでプロセス内にキャッシュする（マウント状態が変わっても
// 数秒で反映される）。
const mountCacheTTL = 2 * time.Second

var (
	mountCacheMu  sync.Mutex
	mountCacheOut string
	mountCacheOK  bool
	mountCacheAt  time.Time
)

// mountOutput は mount の出力を TTL 付きで返す。取得できたかを ok で返す。
func mountOutput() (out string, ok bool) {
	mountCacheMu.Lock()
	defer mountCacheMu.Unlock()
	if !mountCacheAt.IsZero() && time.Since(mountCacheAt) < mountCacheTTL {
		return mountCacheOut, mountCacheOK
	}
	data, err := exec.Command("mount").Output()
	if err != nil {
		mountCacheOut, mountCacheOK = "", false
	} else {
		mountCacheOut, mountCacheOK = string(data), true
	}
	mountCacheAt = time.Now()
	return mountCacheOut, mountCacheOK
}

// resetMountCache はキャッシュを破棄する（テスト用）。
func resetMountCache() {
	mountCacheMu.Lock()
	mountCacheAt = time.Time{}
	mountCacheMu.Unlock()
}

// ---------------------------------------------------------------------------
// マウント判定
// ---------------------------------------------------------------------------

// DefaultMountRoot はプラットフォームごとの外付けボリューム基点を返す。
// macOS は /Volumes。Linux はディストリビューションごとに /media/<user> と
// /run/media/<user> に分かれるため、存在する方を優先し、無ければ /media を使う。
// "/" を返すと root 直下の全フォルダをボリュームとして扱ってしまうため避ける。
func DefaultMountRoot() string {
	if runtime.GOOS == "darwin" {
		return "/Volumes"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		user := filepath.Base(home)
		for _, cand := range []string{
			filepath.Join("/run/media", user),
			filepath.Join("/media", user),
		} {
			if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
				return cand
			}
		}
	}
	if fi, err := os.Stat("/media"); err == nil && fi.IsDir() {
		return "/media"
	}
	return "/mnt"
}

// IsMountPoint はパスそのものがマウントポイントかどうか。
//
// `mount` の出力にあればマウントポイント。macOS の /Volumes のように
// firmlink で親と fsid が異なるだけのパスを誤判定しないよう、statfs は
// `mount` が使えない環境のフォールバックにだけ使う。
// （bash 版と同じ表記。Linux の \040 エスケープにも対応する）
func IsMountPoint(path string) bool {
	is, _ := isMountPointKnown(path)
	return is
}

// isMountPointKnown は IsMountPoint の判定と、判定できたかどうかを返す。
// mount も statfs も使えない環境では known=false になる。
func isMountPointKnown(path string) (is, known bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return false, true // 存在しない・ディレクトリでないことは確定
	}
	if path == "/" {
		return true, true
	}
	// mount の出力を優先する。statfs の fsid 比較だけでは、macOS の
	// firmlink（/Volumes など）を実マウントと誤判定してしまう。
	// 出力は TTL 付きでキャッシュし、exec の往復を減らす。
	if out, ok := mountOutput(); ok {
		return containsMountPoint(out, path), true
	}
	return mountPointByFsid(path)
}

// mountPointByFsid は statfs の Fsid を親と比較する。
// 判定できたら (known=true, isMount=...) を返す。statfs が失敗したら known=false。
func mountPointByFsid(path string) (known, isMount bool) {
	parent := filepath.Dir(path)
	var st, pst syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false, false
	}
	if err := syscall.Statfs(parent, &pst); err != nil {
		return false, false
	}
	return true, st.Fsid != pst.Fsid
}

// containsMountPoint は mount の出力に path がマウントポイントとして現れるかを返す。
// " on <path> " または行末（" on <path>\n"）に一致すればマウントポイント。
// path そのものと、エスケープ表記 (\040 など) の両方を照合する。
func containsMountPoint(out, path string) bool {
	for _, p := range []string{path, mountEscape(path)} {
		if strings.Contains(out, " on "+p+" ") || strings.Contains(out, " on "+p+"\n") {
			return true
		}
	}
	return false
}

// mountEscape は mount が使うエスケープ表記へ変換する
// （空白→\040、タブ→\011、改行→\012、バックスラッシュ→\134）。
func mountEscape(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch r {
		case ' ':
			b.WriteString(`\040`)
		case '\t':
			b.WriteString(`\011`)
		case '\n':
			b.WriteString(`\012`)
		case '\\':
			b.WriteString(`\134`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Dirname は親ディレクトリへ1段上がる（bash 版 dirname と同じく、区切りの
// ない相対パス "x" は "." になる）。
func Dirname(p string) string {
	if p == "/" {
		return "/"
	}
	return filepath.Dir(p)
}

// MountPointOf はパス（存在しなければ最も近い既存の祖先）から上へたどり、
// 最初に見つかる実マウントポイントを返す。見つからなければ "/"。
func MountPointOf(p string) string {
	if mp, known := mountPointOf(p); known {
		return mp
	}
	return "/" // 判定できない場合は従来どおり root 扱い
}

// mountPointOf は MountPointOf の本体。判定できたかどうかも返す。
func mountPointOf(p string) (mp string, known bool) {
	p = AbsPath(p) // 相対パスはカレント基準にそろえる（"." のまま上れなくなるのを防ぐ）
	for !exists(p) && p != "/" {
		p = Dirname(p)
	}
	for {
		is, ok := isMountPointKnown(p)
		if !ok {
			return "", false
		}
		if is {
			return p, true
		}
		if p == "/" {
			return "/", true
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
//
// 判定対象は「基点の下にあるパス」だけ:
//   - 基点そのもの（dst == mountRoot）は「下のボリューム」ではないため対象外（true）
//   - mountRoot 自体が実マウントポイントのときは、その直下も対象内として扱う
func DestIsMounted(dst, mountRoot string) bool {
	if mountRoot == "/" {
		return true
	}
	if dst == mountRoot {
		return true // 基点そのものはボリューム判定の対象外
	}
	if !strings.HasPrefix(dst, mountRoot+"/") {
		return true // 基点の下ではない → 対象外
	}
	mp, known := mountPointOf(dst)
	if !known {
		// マウント状態を判定できない環境（mount が無い等）では、利用者の
		// 正当なコピーをブロックしないようマウント済みとして扱う。
		return true
	}
	return destIsMountedAt(mp, mountRoot)
}

// destIsMountedAt は、コピー先の実マウントポイント mp が基点 mountRoot の
// 下（または基点そのもの）かを返す純関数。mp == mountRoot を含めることで、
// 基点自体が実マウントポイントのときにその直下を「未マウント」と誤判定しない。
func destIsMountedAt(mp, mountRoot string) bool {
	return mp == mountRoot || strings.HasPrefix(mp, mountRoot+"/")
}

// ---------------------------------------------------------------------------
// 空き容量
// ---------------------------------------------------------------------------

// FreeBytes は指定パス（存在しなければ最も近い既存の祖先）の空き容量をバイトで返す。
// 取得できない場合は 0（不明と 0 バイトの区別が必要なときは FreeBytesOK を使う）。
func FreeBytes(p string) int64 {
	free, _ := FreeBytesOK(p)
	return free
}

// FreeBytesOK は空き容量と、取得できたかどうかを返す。
// 取得できない場合に 0 を「空き 0 バイト」と誤解すると、コピーを誤って
// 中止したり、逆に空き不足を見落としたりするため区別できるようにする。
func FreeBytesOK(p string) (int64, bool) {
	for !exists(p) && p != "/" {
		p = Dirname(p)
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}

// TotalBytes は指定パスの総容量をバイトで返す。取得できない場合は 0
// （不明と 0 バイトの区別が必要なときは TotalBytesOK を使う）。
func TotalBytes(p string) int64 {
	total, _ := TotalBytesOK(p)
	return total
}

// TotalBytesOK は総容量と、取得できたかどうかを返す。
func TotalBytesOK(p string) (int64, bool) {
	for !exists(p) && p != "/" {
		p = Dirname(p)
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0, false
	}
	return int64(st.Blocks) * int64(st.Bsize), true
}

// ---------------------------------------------------------------------------
// 走査（ファイル数・合計バイト数）
// ---------------------------------------------------------------------------

// ScanResult は exclude を適用した走査の結果。
type ScanResult struct {
	Files int
	Bytes int64
}

// ScanDir は dir を走査し、除外パターンに一致する場所をまるごと枝ごと落として
// （bash 版 find -prune と同じ）、ファイル数と合計バイト数を返す。走査中の
// エラー（権限など）は無視する。
//
// パターンの解釈は rsync の --exclude に寄せている（MatchExclude を参照）。
// 以前はベース名への filepath.Match だけだったため、"sub/cache" や "cache/"、
// "**" のようなパス依存のパターンで、転送（rsync）と走査の除外がずれていた。
//
// excludes はそのまま適用される（既定除外も含めた全パターンを渡す）。
// Job からは Job.AllExcludePatterns() を渡す（既定除外の二重適用を防ぐ）。
func ScanDir(dir string, excludes []string) ScanResult {
	var res ScanResult
	// リモート先はローカルからは走査できない（host:path を WalkDir しても失敗する）。
	// 呼び出し側は 0 件として扱い、進捗は rsync の出力から数える。
	if IsRemote(dir) {
		return res
	}
	excl := make([]string, 0, len(excludes))
	for _, e := range excludes {
		if e != "" {
			excl = append(excl, e)
		}
	}

	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == dir {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			rel = d.Name()
		}
		rel = filepath.ToSlash(rel)
		for _, pat := range excl {
			if MatchExclude(pat, rel, d.IsDir()) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() {
			// bash 版の find -type f と同じく通常ファイルだけを数える
			// （シンボリックリンク・FIFO・デバイスファイルなどは含めない）
			if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
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

// DfHuman は df -h と同等の情報を得る（--status の表示用）。取得できなければ空文字を返す。
//
// 列の位置を決め打ちしないのは、環境によって出力が変わるため:
// デバイス名に空白が含まれれば列がずれ、GNU df は長いデバイス名を折り返して
// Size 列だけを行末へ送ることがある。そこで「サイズらしい列が3つ並ぶ位置」を
// 出力全体から探す（先頭が Size / Used / Avail になる）。
func DfHuman(path string) (size, used, avail string) {
	out, err := exec.Command("df", "-h", path).Output()
	if err != nil {
		return "", "", ""
	}
	return parseDfValues(string(out))
}

// parseDfValues は df -h の出力から Size / Used / Avail の3列を取り出す。
func parseDfValues(out string) (size, used, avail string) {
	f := strings.Fields(out)
	for i := 0; i+2 < len(f); i++ {
		if isDfSize(f[i]) && isDfSize(f[i+1]) && isDfSize(f[i+2]) {
			return f[i], f[i+1], f[i+2]
		}
	}
	return "", "", ""
}

// isDfSize は df のサイズ列らしい文字列かを返す（"1.8Ti" "512B" "0Bi" "1024" など）。
// 単位は英字 1〜2 文字までに限り、ヘッダの "512-blocks" や "%iused" のような
// 列、Use% の "79%" を除外する。
func isDfSize(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		j := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == j {
			return false
		}
	}
	unit := s[i:]
	if len(unit) > 2 {
		return false
	}
	for k := 0; k < len(unit); k++ {
		c := unit[k]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}
