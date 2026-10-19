package core

// remote_test.go - リモートパス（[user@]host:path / rsync://）の回帰テスト。
//   * リモート先にローカルの迷子ディレクトリ（"user@host:"）を作らない
//   * リモート元を「コピー元が存在しません」で弾かない
//   * リモートはファイル数ベースで進捗し、集計をスキップする
//
// rsync 本体はテスト用の偽スクリプト（PATH 先頭）に差し替える。

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFakeRsync はテスト用の rsync スクリプトを PATH に差し込む。
// dryScript は dry-run（-n 付き）呼び出し時の標準出力（差分の偽装）。
func installFakeRsync(t *testing.T, dryScript string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"-n\" ]; then\n" +
		dryScript +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "rsync"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// chdirTemp はテスト中だけカレントを一時ディレクトリへ移す（相対パス生成の確認用）。
func chdirTemp(t *testing.T) string {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func TestEnsureLocalDestRemote(t *testing.T) {
	cwd := chdirTemp(t)
	if err := ensureLocalDest("user@host:/data/backup"); err != nil {
		t.Fatalf("リモート先でエラー: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "user@host:")); !os.IsNotExist(err) {
		t.Fatal("リモート先なのにローカルへディレクトリを作った（迷子フォルダ）")
	}
}

func TestCopyOneRemoteDstNoStrayDir(t *testing.T) {
	// dry-run では「>f+++++++++ 5.00 a.txt」形式の差分を返す（PlanChanges 用）。
	installFakeRsync(t, "    echo '>f+++++++++ 5.00 a.txt'\n")
	cwd := chdirTemp(t)

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := CopyOne(&Job{Src: src, Dst: "user@host:/data/backup", FAT: true, Out: &out, Err: &errb})
	if rc != 0 {
		t.Fatalf("CopyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "✓ コピー完了") {
		t.Errorf("完了表示がない:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "user@host:")); !os.IsNotExist(err) {
		t.Fatal("リモート先なのにローカルへディレクトリを作った（迷子フォルダ）")
	}
}

func TestCopyOneRemoteSrc(t *testing.T) {
	installFakeRsync(t, "    echo '>f+++++++++ 5.00 a.txt'\n")

	dst := t.TempDir()
	var out, errb bytes.Buffer
	rc := CopyOne(&Job{Src: "user@host:/data/backup", Dst: dst, FAT: true, Out: &out, Err: &errb})
	if rc != 0 {
		t.Fatalf("リモート元で失敗してはいけない: rc=%d err=%s", rc, errb.String())
	}
	if strings.Contains(errb.String(), "コピー元が存在しません") {
		t.Errorf("リモート元をローカル stat で弾いている: %s", errb.String())
	}
}

func TestVerifyOneRemoteSkipsAggregate(t *testing.T) {
	installFakeRsync(t, "") // 差分なし

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := VerifyOne(&Job{Src: src, Dst: "user@host:/data/backup", FAT: true, Out: &out, Err: &errb})
	if rc != 0 {
		t.Fatalf("VerifyOne = %d, err=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "✓ 照合 OK") {
		t.Errorf("照合 OK が出ない:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "ファイル数の集計は省略") {
		t.Errorf("リモートなのに集計を出している:\n%s", out.String())
	}
	if strings.Contains(errb.String(), "存在しません") {
		t.Errorf("リモート先をローカル stat で弾いている: %s", errb.String())
	}
}

// ":" を含むローカルのパスに「./ を付けよ」のヒントを出すこと。
// リテラルのパスが実在するときだけ出す（本物のリモートには出さない）。
func TestColonLocalHint(t *testing.T) {
	chdirTemp(t)
	if err := os.Mkdir("photos:2019", 0o755); err != nil {
		t.Fatal(err)
	}

	if h := ColonLocalHint("photos:2019"); !strings.Contains(h, "./photos:2019") {
		t.Errorf("ローカル誤認のヒントが出ない: %q", h)
	}
	// "./" 付きはローカル扱いなのでヒントなし
	if h := ColonLocalHint("./photos:2019"); h != "" {
		t.Errorf("エスケープ済みにヒントを出した: %q", h)
	}
	// 実在しないリモート（host:path）にはヒントなし
	if h := ColonLocalHint("user@nas:/backup"); h != "" {
		t.Errorf("存在しないリモートにヒントを出した: %q", h)
	}
	// ローカルパスにはヒントなし
	if h := ColonLocalHint("/Volumes/HDD"); h != "" {
		t.Errorf("ローカルパスにヒントを出した: %q", h)
	}
}

func TestScanDirRemoteIsZero(t *testing.T) {
	if res := ScanDir("user@host:/data", nil); res.Files != 0 || res.Bytes != 0 {
		t.Errorf("リモートは走査できない（0 のはず）: %+v", res)
	}
}

func TestRsyncArgsProgressFiles(t *testing.T) {
	j := &Job{Src: "a", Dst: "user@host:/b", ProgressFiles: true}
	args := j.rsyncArgs()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--out-format=%i %l %n") {
		t.Errorf("進捗は %%i %%l %%n を使うはず: %v", args)
	}
	if strings.Contains(joined, " -q ") || args[len(args)-1] == "-q" {
		t.Errorf("出力を使うときは -q を付けない: %v", args)
	}
	// ListFiles と ProgressFiles が同時でも同じ out-format を使える
	j2 := &Job{Src: "a", Dst: "user@host:/b", ListFiles: true, ProgressFiles: true}
	if got := strings.Join(j2.rsyncArgs(), " "); !strings.Contains(got, "--out-format=%i %l %n") {
		t.Errorf("共通の out-format を使うはず: %v", j2.rsyncArgs())
	}
	// ShowProgress も %l を読むために out-format を付ける
	j3 := &Job{Src: "a", Dst: "/b", ShowProgress: true}
	if got := strings.Join(j3.rsyncArgs(), " "); !strings.Contains(got, "--out-format=%i %l %n") {
		t.Errorf("ShowProgress でも out-format を付けるはず: %v", j3.rsyncArgs())
	}
}
