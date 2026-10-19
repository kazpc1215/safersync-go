package core

// helpers_test.go - パス・ディスク・進捗表示の純関数とコピー本体のテスト
// （カバレッジ強化。bash 版の同名ヘルパとの対応を確認する）

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// パス helpers
// ---------------------------------------------------------------------------

func TestExpandTilde(t *testing.T) {
	t.Setenv("HOME", "/tmp/ss-home")
	cases := []struct{ in, want string }{
		{"~", "/tmp/ss-home"},
		{"~/photos", "/tmp/ss-home/photos"},
		{"/a/b", "/a/b"},
		{"~user/x", "~user/x"}, // 他ユーザーの ~ は展開しない（bash 版と同じ）
		{"a~b", "a~b"},
	}
	for _, c := range cases {
		if got := ExpandTilde(c.in); got != c.want {
			t.Errorf("ExpandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAbsPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/ss-home")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ in, want string }{
		{"~", "/tmp/ss-home"},
		{"~/data", "/tmp/ss-home/data"},
		{"/abs/path/", "/abs/path"}, // 末尾スラッシュは落とす
		{"/", "/"},
		{"rel/dir", filepath.Join(wd, "rel/dir")},
	}
	for _, c := range cases {
		if got := AbsPath(c.in); got != c.want {
			t.Errorf("AbsPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsRemote(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"user@host:/data", true},
		{"host:path", true},
		{"rsync://host/module", true},
		{"a:b/c", true},
		{"/Volumes/HDD", false},
		{"relative/path", false},
		{"a/b:c", false}, // "/" より後ろの ":" はリモート扱いしない
	}
	for _, c := range cases {
		if got := IsRemote(c.in); got != c.want {
			t.Errorf("IsRemote(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// コピー先がコピー元の中にある事故（入れ子コピー）を検出できること。
func TestIsInside(t *testing.T) {
	cases := []struct {
		parent, child string
		want          bool
	}{
		{"/a", "/a", true},
		{"/a", "/a/b", true},
		{"/a", "/a/b/c", true},
		{"/a/", "/a/b/", true},
		{"/a", "/a/b/../b", true},
		{"/a", "/ab", false},                // 部分文字列ではない
		{"/a/b", "/a", false},               // 逆方向
		{"/a", "/b", false},                 // 兄弟
		{"/a", "/", false},                  // 親は内側ではない
		{"/", "/a", true},                   // ルートはすべての内側（内蔵ディスク全体 → 任意の場所）
		{"/Volumes/A", "/Volumes/B", false}, // 別ボリューム（同列）は対象外
		{"", "/a/b", false},                 // 空は判定しない
		{"/a", "", false},
	}
	for _, c := range cases {
		if got := IsInside(c.parent, c.child); got != c.want {
			t.Errorf("IsInside(%q, %q) = %v, want %v", c.parent, c.child, got, c.want)
		}
	}
}

// 相対パスはカレント基準の絶対パスとして親子判定されること。
func TestIsInsideRelative(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !IsInside("sub", "sub/deeper") {
		t.Errorf("相対パスの内側判定ができていない (wd=%s)", wd)
	}
	if IsInside("sub", "other") {
		t.Errorf("無関係な相対パスを内側と誤判定した (wd=%s)", wd)
	}
}

// シンボリックリンク経由でも「実は内側／外側」を取り違えないこと。
func TestIsInsideSymlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "photos")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("シンボリックリンクを作成できないためスキップ: %v", err)
	}

	// link は real を指すので link/backup は real の内側
	if !IsInside(real, filepath.Join(link, "backup")) {
		t.Error("シンボリックリンク経由の内側を見落とした")
	}

	// src 内のリンクが外を指す場合は内側ではない（入れ子にならない）
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	linkOut := filepath.Join(real, "to-outside")
	if err := os.Symlink(outside, linkOut); err != nil {
		t.Skipf("シンボリックリンクを作成できないためスキップ: %v", err)
	}
	if IsInside(real, filepath.Join(linkOut, "backup")) {
		t.Error("外を指すリンクを内側と誤判定した")
	}
}

// Linux の mount が出すエスケープ表記（空白→\040）でもマウントポイントを検出できること。
func TestContainsMountPoint(t *testing.T) {
	out := "/dev/sda1 on / type ext4 (rw)\n" +
		"/dev/sdb1 on /media/My\\040Disk type ext4 (rw)\n" +
		"/dev/sdc1 on /mnt/data type ext4 (rw)\n"

	if !containsMountPoint(out, "/") {
		t.Error("/ を検出できない")
	}
	if !containsMountPoint(out, "/mnt/data") {
		t.Error("通常のマウントポイントを検出できない")
	}
	// 空白は \040 にエスケープされて出力される
	if !containsMountPoint(out, "/media/My Disk") {
		t.Error("空白を含むマウントポイント（\\040 表記）を検出できない")
	}
	if containsMountPoint(out, "/mnt/other") {
		t.Error("マウントされていないパスをマウントポイントと誤判定した")
	}
}

func TestMountEscape(t *testing.T) {
	cases := map[string]string{
		"/media/My Disk": "/media/My\\040Disk",
		"/a\tb":          `/a\011b`,
		"/a\\b":          `/a\134b`,
		"/plain":         "/plain",
	}
	for in, want := range cases {
		if got := mountEscape(in); got != want {
			t.Errorf("mountEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// ディスク関連
// ---------------------------------------------------------------------------

func TestDirname(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/", "/"},
		{"/a", "/"},
		{"/a/b", "/a"},
		{"/a/b/", "/a/b"}, // bash の dirname と同じ（末尾スラッシュは最後の要素を指す）
		{"a/b", "a"},
		{"x", "."},
	}
	for _, c := range cases {
		if got := Dirname(c.in); got != c.want {
			t.Errorf("Dirname(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMountPointOf(t *testing.T) {
	if got := MountPointOf("/"); got != "/" {
		t.Errorf("MountPointOf(/) = %q, want /", got)
	}

	// 存在しないパスでも、近い既存の祖先から上ってマウントポイントを見つける
	p := filepath.Join(t.TempDir(), "not", "yet")
	got := MountPointOf(p)
	if !IsMountPoint(got) {
		t.Errorf("MountPointOf(%q) = %q（マウントポイントでない）", p, got)
	}
	if !strings.HasPrefix(p, got) {
		t.Errorf("MountPointOf(%q) = %q（祖先でない）", p, got)
	}

	// 相対パスでも止まらない（"." で無限ループしない）
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := MountPointOf("."); !IsMountPoint(got) || !strings.HasPrefix(wd, got) {
		t.Errorf("MountPointOf(\".\") = %q（マウントポイントでない/祖先でない）", got)
	}
}

func TestDestIsMounted(t *testing.T) {
	if !DestIsMounted("/any/where", "/") {
		t.Error("mountRoot=/ は常にマウント扱いのはず")
	}
	if !DestIsMounted("/tmp/x", "/Volumes") {
		t.Error("基点の外は判定対象外（true）のはず")
	}

	root := filepath.Join(t.TempDir(), "vols")
	card := filepath.Join(root, "card")
	if err := os.MkdirAll(card, 0o755); err != nil {
		t.Fatal(err)
	}
	// 基点の下だが実マウントされていない（t.TempDir はマウントポイントではない）
	if DestIsMounted(card, root) {
		t.Errorf("未マウントの %s をマウント済みと判定した", card)
	}
	if !DestIsMounted(root, root) {
		t.Error("基点そのものは判定対象外のはず")
	}

	// 実マウントポイントと基点の関係（純関数）。基点自体がマウントポイントの
	// ときも、その直下を未マウントと誤判定しない。
	cases := []struct {
		mp, base string
		want     bool
	}{
		{"/Volumes/HDD", "/Volumes", true},
		{"/Volumes/HDD/sub", "/Volumes", true},
		{"/Volumes", "/Volumes", true}, // 基点自体がマウントポイント
		{"/", "/Volumes", false},       // 基点の下に実マウントが無い
		{"/mnt", "/Volumes", false},
	}
	for _, c := range cases {
		if got := destIsMountedAt(c.mp, c.base); got != c.want {
			t.Errorf("destIsMountedAt(%q, %q) = %v, want %v", c.mp, c.base, got, c.want)
		}
	}
}

func TestVolumes(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"disk1", "disk2"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// リンク切れは含めない・ディレクトリへのリンクは含める（bash 版と同じ）
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "disk1"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	got := Volumes(root)
	want := []string{
		filepath.Join(root, "disk1"),
		filepath.Join(root, "disk2"),
		filepath.Join(root, "link"),
	}
	if len(got) != len(want) {
		t.Fatalf("Volumes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Volumes[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if v := Volumes(filepath.Join(root, "nope")); v != nil {
		t.Errorf("存在しない基点は nil のはず: %v", v)
	}
}

func TestDfHuman(t *testing.T) {
	size, used, avail := DfHuman("/")
	if size == "" || used == "" || avail == "" {
		t.Errorf("DfHuman(/) が空: %q %q %q", size, used, avail)
	}
	if s, _, _ := DfHuman(filepath.Join(t.TempDir(), "nope")); s != "" {
		t.Errorf("存在しないパスは空のはず: %q", s)
	}
}

// df -h の列位置は環境で変わる（デバイス名の空白・折り返し）。位置決め打ちを
// せず、サイズらしい列を探す実装になっていることを確かめる。
func TestParseDfValues(t *testing.T) {
	cases := []struct {
		name              string
		out               string
		size, used, avail string
	}{
		{
			name: "macOS",
			out:  "Filesystem Size Used Avail Capacity iused ifree %iused Mounted on\n/dev/disk1s1s1 1.8Ti 466Gi 112Mi 79% 12345 6789 1% /\n",
			size: "1.8Ti", used: "466Gi", avail: "112Mi",
		},
		{
			name: "GNU df の長いデバイス名で折り返し",
			out:  "Filesystem Size Used Avail Use% Mounted on\n/dev/mapper/really-long-device-name\n 100G 50G 40G 56% /mnt/data\n",
			size: "100G", used: "50G", avail: "40G",
		},
		{
			name: "デバイス名に空白",
			out:  "Filesystem Size Used Avail Capacity Mounted on\n//nas/My Share 2.0Ti 1.0Ti 1.0Ti 50% /Volumes/My Share\n",
			size: "2.0Ti", used: "1.0Ti", avail: "1.0Ti",
		},
		{name: "ヘッダのみ", out: "Filesystem Size Used Avail Use% Mounted on\n"},
		{name: "空", out: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			size, used, avail := parseDfValues(c.out)
			if size != c.size || used != c.used || avail != c.avail {
				t.Errorf("parseDfValues = (%q,%q,%q), want (%q,%q,%q)",
					size, used, avail, c.size, c.used, c.avail)
			}
		})
	}
}

func TestIsDfSize(t *testing.T) {
	yes := []string{"1.8Ti", "512B", "1024", "0Bi", "1.5", "40G"}
	no := []string{"79%", "512-blocks", "%iused", "Filesystem", "1.", "", "Gi", "1.5TiB", "-1G"}
	for _, s := range yes {
		if !isDfSize(s) {
			t.Errorf("isDfSize(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isDfSize(s) {
			t.Errorf("isDfSize(%q) = true, want false", s)
		}
	}
}

func TestTotalBytes(t *testing.T) {
	if got := TotalBytes(t.TempDir()); got <= 0 {
		t.Errorf("TotalBytes = %d, want > 0", got)
	}
	// 存在しないパスでも近い既存の祖先から取れる
	if got := TotalBytes(filepath.Join(t.TempDir(), "x", "y")); got <= 0 {
		t.Errorf("存在しないパスの TotalBytes = %d, want > 0", got)
	}
}

// ---------------------------------------------------------------------------
// 進捗表示
// ---------------------------------------------------------------------------

func TestProgressWidth(t *testing.T) {
	cases := []struct {
		cols string
		want int
	}{
		{"", 8}, {"abc", 8}, {"0", 8}, {"60", 0}, {"80", 8}, {"95", 16}, {"120", 24},
	}
	for _, c := range cases {
		t.Setenv("COLUMNS", c.cols)
		if got := progressWidth(); got != c.want {
			t.Errorf("COLUMNS=%q: progressWidth = %d, want %d", c.cols, got, c.want)
		}
	}
}

func TestDrawProgress(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	s := DrawProgress(50, 1024, 2048, 512, 4)
	for _, want := range []string{"[", "█", "░", "50%", "残り 約 4秒"} {
		if !strings.Contains(s, want) {
			t.Errorf("DrawProgress に %q がない: %q", want, s)
		}
	}

	// 狭い端末ではバーを出さない（折り返して崩れるため）
	t.Setenv("COLUMNS", "40")
	s = DrawProgress(0, 0, 0, 0, -1)
	if strings.Contains(s, "[") {
		t.Errorf("狭い端末でバーを出している: %q", s)
	}
	if !strings.Contains(s, "残り --") {
		t.Errorf("ETA 不明の表示がない: %q", s)
	}

	// 100% では残り時間を出さない
	t.Setenv("COLUMNS", "120")
	s = DrawProgress(100, 100, 100, 10, 0)
	if strings.Contains(s, "残り") {
		t.Errorf("100%% で残り時間を出している: %q", s)
	}
}

// リモート先の進捗（件数ベース）の描画。バイト版と同じ体裁で件数・件/秒を出す。
func TestDrawProgressFiles(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	s := DrawProgressFiles(50, 3, 6, 1.5, 2)
	for _, want := range []string{"[", "█", "░", "50%", "3 / 6 ファイル", "1.5件/秒", "残り 約 2秒"} {
		if !strings.Contains(s, want) {
			t.Errorf("DrawProgressFiles に %q がない: %q", want, s)
		}
	}

	// 狭い端末ではバーを出さない（折り返して崩れるため）
	t.Setenv("COLUMNS", "40")
	s = DrawProgressFiles(0, 0, 0, 0, -1)
	if strings.Contains(s, "[") {
		t.Errorf("狭い端末でバーを出している: %q", s)
	}
	if !strings.Contains(s, "残り --") {
		t.Errorf("ETA 不明の表示がない: %q", s)
	}

	// 100% では残り時間を出さない
	t.Setenv("COLUMNS", "120")
	s = DrawProgressFiles(100, 6, 6, 2, 0)
	if strings.Contains(s, "残り") {
		t.Errorf("100%% で残り時間を出している: %q", s)
	}
}

// countTransferredFiles は rsync の --out-format=%i から「転送された通常ファイル」
// （行頭 ">f"）だけを数えること。ディレクトリ作成や属性のみの変化は数えない。
func TestCountTransferredFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	content := strings.Join([]string{
		"cd+++++++++ ./",
		">f+++++++++ 5.00 a.txt",
		".f...p..... b.txt", // 属性のみ → 数えない
		">f         c.txt",  // 形式ゆれでも ">f" なら数える
		">f+++++++++ d/e.txt",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countTransferredFiles(path); got != 3 {
		t.Errorf("countTransferredFiles = %d, want 3", got)
	}
	// 読めないファイルは 0（進捗は 0 件として扱う）
	if got := countTransferredFiles(filepath.Join(t.TempDir(), "missing")); got != 0 {
		t.Errorf("存在しないファイルで %d, want 0", got)
	}
}

// 追記分だけを数える（毎tickで全読みしない）。改行で終わっていない行は
// 完成するまで数えない。
func TestTransferredCounterIncremental(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	// 1件は改行で終わっていない（未完）
	if err := os.WriteFile(path, []byte(">f++++ a\n>f++++ b"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &transferredCounter{path: path}
	if got := c.countTransferred(); got != 1 {
		t.Errorf("1回目 = %d, want 1（未完の行は数えない）", got)
	}
	// 未完の行が完成し、さらに1件追記される
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n>f++++ c\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := c.countTransferred(); got != 3 {
		t.Errorf("2回目 = %d, want 3", got)
	}
	if got := c.countTransferred(); got != 3 {
		t.Errorf("追記なしの3回目 = %d, want 3（二重に数えない）", got)
	}
}

func TestProgressCountFrom(t *testing.T) {
	start := time.Unix(1000, 0)
	now := time.Unix(1010, 0) // 10 秒経過

	// 100 / 200 件 = 50%、速度 10 件/秒、残り 10 秒
	if p, speed, eta := progressCountFrom(100, 200, start, now); p != 50 || speed != 10 || eta != 10 {
		t.Errorf("progressCountFrom = (%d, %v, %d), want (50, 10, 10)", p, speed, eta)
	}
	// 分母不明（0）は 100% 扱い・残りなし
	if p, _, eta := progressCountFrom(0, 0, start, now); p != 100 || eta != -1 {
		t.Errorf("plan=0: p=%d eta=%d, want 100, -1", p, eta)
	}
	// 分母を超えても 100% で止まる
	if p, _, _ := progressCountFrom(50, 20, start, now); p != 100 {
		t.Errorf("done>plan: p=%d, want 100", p)
	}
	// 経過 1 秒未満は 1 秒として扱う（0 除算しない）
	if _, speed, _ := progressCountFrom(100, 200, now, now); speed != 100 {
		t.Errorf("elapsed<1: speed=%v, want 100", speed)
	}
	// 小数の速度（2 秒で 1 件 = 0.5 件/秒）も表現できる
	if _, speed, _ := progressCountFrom(1, 200, start, time.Unix(1002, 0)); speed != 0.5 {
		t.Errorf("fractional speed: %v, want 0.5", speed)
	}
}

func TestRunWithProgress(t *testing.T) {
	var out, errb bytes.Buffer
	j := &Job{Src: ".", Dst: ".", Out: &out, Err: &errb}

	// 子プロセスの stdout / stderr が転送される（一時ファイル経由で最後に出力）
	cmd := exec.Command("sh", "-c", "printf hello; printf oops >&2")
	if rc := runWithProgress(cmd, j); rc != 0 {
		t.Fatalf("rc = %d, want 0（err=%s）", rc, errb.String())
	}
	if s := out.String(); !strings.Contains(s, "hello") || !strings.Contains(s, "oops") {
		t.Errorf("子プロセスの出力が転送されていない: %q", s)
	}

	// 終了コードはそのまま返す
	cmd2 := exec.Command("sh", "-c", "exit 7")
	if rc := runWithProgress(cmd2, j); rc != 7 {
		t.Errorf("rc = %d, want 7", rc)
	}
}

// ---------------------------------------------------------------------------
// コピー本体（rsync が必要）
// ---------------------------------------------------------------------------

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCopyOneSuccessAndVerify(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "backup")
	writeTestFile(t, filepath.Join(src, "a.txt"), "hello")
	writeTestFile(t, filepath.Join(src, "sub", "b.bin"), "world!")

	var out, errb bytes.Buffer
	j := &Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errb}
	if rc := CopyOne(j); rc != 0 {
		t.Fatalf("CopyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "コピーするファイル: 2 件") {
		t.Errorf("件数の表示がおかしい:\n%s", out.String())
	}
	if j.TotalFiles != 2 || j.TotalBytes <= 0 {
		t.Errorf("累計 = %d files / %d bytes, want 2 files / >0 bytes", j.TotalFiles, j.TotalBytes)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "sub", "b.bin")); err != nil || string(b) != "world!" {
		t.Errorf("コピーされていない: %v / %q", err, b)
	}

	// 2 回目は差分なし
	out.Reset()
	errb.Reset()
	j2 := &Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errb}
	if rc := CopyOne(j2); rc != 0 {
		t.Fatalf("2回目 CopyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "転送するものはありません") {
		t.Errorf("差分なしの表示がない:\n%s", out.String())
	}

	// 照合 OK
	var vout, verr bytes.Buffer
	j3 := &Job{Src: src, Dst: dst, FAT: true, Out: &vout, Err: &verr}
	if rc := VerifyOne(j3); rc != 0 {
		t.Fatalf("VerifyOne = %d, err=%s", rc, verr.String())
	}
	if !strings.Contains(vout.String(), "✓ 照合 OK") {
		t.Errorf("照合 OK の表示がない:\n%s", vout.String())
	}
}

func TestCopyOneDryRun(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "a.txt"), "hello")
	dst := filepath.Join(t.TempDir(), "backup")

	var out, errb bytes.Buffer
	j := &Job{Src: src, Dst: dst, FAT: true, DryRun: true, Out: &out, Err: &errb}
	if rc := CopyOne(j); rc != 0 {
		t.Fatalf("CopyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "dry-run のため実際にはコピーしません") {
		t.Errorf("dry-run の表示がない:\n%s", out.String())
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("dry-run なのにコピー先が作られている: %v", err)
	}
}

// --progress 相当（runWithProgress 経由）でもコピーできること
func TestCopyOneWithProgress(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "a.txt"), "hello")
	dst := filepath.Join(t.TempDir(), "backup")

	var out, errb bytes.Buffer
	j := &Job{Src: src, Dst: dst, FAT: true, ShowProgress: true, Out: &out, Err: &errb}
	if rc := CopyOne(j); rc != 0 {
		t.Fatalf("CopyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "✓ コピー完了") {
		t.Errorf("完了表示がない:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Errorf("コピーされていない: %v", err)
	}
}

// progressFrom は走査済みバイト数から進捗を計算する純粋関数（非同期走査の基盤）。
// 分母は「開始時の転送先(base) + 今回の転送量(plan)」。
func TestProgressFrom(t *testing.T) {
	start := time.Unix(1000, 0)
	now := time.Unix(1010, 0) // 10 秒
	// base=50, plan=200, done=100 → gained=50 → 25%、速度 5 B/s、残り 30 秒
	p, done, speed, eta := progressFrom(100, 50, 200, start, now)
	if p != 25 || done != 100 || speed != 5 || eta != 30 {
		t.Errorf("progressFrom = (%d, %d, %d, %d), want (25, 100, 5, 30)", p, done, speed, eta)
	}
	// 差分コピー: 転送先に既に大量にあっても、今回の転送量を基準に 0% から始まる
	if p, _, _, _ := progressFrom(1_000_000, 1_000_000, 100, start, now); p != 0 {
		t.Errorf("計画基準で 0%% から始まるはず: p=%d", p)
	}
	// plan 不明（0）は 100%・残りなし
	if p, _, _, eta := progressFrom(0, 0, 0, start, now); p != 100 || eta != -1 {
		t.Errorf("plan=0: p=%d eta=%d, want 100, -1", p, eta)
	}
	// base より減っていても速度を負にしない
	if _, _, speed, eta := progressFrom(100, 500, 200, start, now); speed != 0 || eta != -1 {
		t.Errorf("減少時: speed=%d eta=%d, want 0, -1", speed, eta)
	}
	// 計画を超えても 100% で止まる
	if p, _, _, _ := progressFrom(500, 50, 200, start, now); p != 100 {
		t.Errorf("done>plan: p=%d, want 100", p)
	}
	// 経過 1 秒未満は 1 秒として扱う（0 除算しない）
	if _, _, speed, _ := progressFrom(150, 50, 200, now, now); speed != 100 {
		t.Errorf("elapsed<1: speed=%d, want 100", speed)
	}
}

// --checksum は実コピーでも rsync に渡す（同サイズ・同 mtime の内容違いを検出する）。
func TestCopyOneChecksumDetectsSameSizeSameTime(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	srcFile := filepath.Join(src, "f.txt")
	dstFile := filepath.Join(dst, "f.txt")
	if err := os.WriteFile(srcFile, []byte("aaaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dstFile, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(srcFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dstFile, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	j := &Job{Src: src, Dst: dst, FAT: true, Checksum: true, Out: &out, Err: &errb}
	if rc := CopyOne(j); rc != 0 {
		t.Fatalf("CopyOne = %d, err=%s", rc, errb.String())
	}
	if b, err := os.ReadFile(dstFile); err != nil || string(b) != "aaaa" {
		t.Fatalf("チェックサム指定でも更新されていない: %v / %q", err, b)
	}
}

// シンボリックリンク・空ディレクトリだけの差分でも CopyOne が取りこぼさない。
func TestCopyOneNonRegularChanges(t *testing.T) {
	if !RsyncAvailable() {
		t.Skip("rsync が見つからないためスキップ")
	}
	src, dst := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(src, "link")); err != nil {
		t.Skipf("シンボリックリンクを作成できないためスキップ: %v", err)
	}
	plan := (&Job{Src: src, Dst: dst, FAT: true}).PlanChanges()
	if plan.Files != 0 || plan.Changes < 2 {
		t.Fatalf("plan = %+v, want Files=0 / Changes>=2", plan)
	}
	var out, errb bytes.Buffer
	j := &Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errb}
	if rc := CopyOne(j); rc != 0 {
		t.Fatalf("CopyOne = %d, err=%s", rc, errb.String())
	}
	if target, err := os.Readlink(filepath.Join(dst, "link")); err != nil || target != "target" {
		t.Errorf("シンボリックリンクがコピーされていない: %v / %q", err, target)
	}
	if fi, err := os.Stat(filepath.Join(dst, "empty")); err != nil || !fi.IsDir() {
		t.Errorf("空ディレクトリがコピーされていない: %v", err)
	}

	// 2回目は .d 以外の差分が無いのでスキップする。
	out.Reset()
	plan2 := (&Job{Src: src, Dst: dst, FAT: true}).PlanChanges()
	if plan2.Changes != 0 {
		t.Fatalf("2回目の Changes = %d（詳細: %v）", plan2.Changes, plan2.Lines)
	}
	if rc := CopyOne(&Job{Src: src, Dst: dst, FAT: true, Out: &out, Err: &errb}); rc != 0 {
		t.Fatalf("2回目 CopyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "転送するものはありません") {
		t.Errorf("2回目が差分なしになっていない:\n%s", out.String())
	}
}
