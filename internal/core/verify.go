package core

// verify.go - データ照合エンジン（bash 版 lib/core/verify.sh の Go 移植）

import (
	"fmt"
	"os"
	"strings"
)

// VerifyOne はコピー先とコピー元を照合する。戻り値は終了コード（0 / 1）。
func VerifyOne(j *Job) int {
	if !isDir(j.Src) {
		fmt.Fprintf(j.Err, "✗ コピー元が存在しません: %s\n", j.Src)
		return 1
	}
	if !isDir(j.Dst) {
		fmt.Fprintf(j.Err, "✗ コピー先が存在しません: %s\n", j.Dst)
		return 1
	}

	// --delete を付けないので、転送先の余分なファイルは差分として出ない
	raw, err := j.CollectDiff("%i %n")
	if err != nil {
		// 差分を取れない状態で「照合 OK」と報告すると、コピー元を消してよいと
		// 誤解させてしまうため、必ず失敗として扱う（終了コード 3）。
		fmt.Fprintf(j.Err, "✗ 照合できませんでした: %s\n", err)
		fmt.Fprintln(j.Err, "✗ 照合ができていないため、コピー元のデータは削除しないでください。")
		j.log(fmt.Sprintf("VERIFY failed %s vs %s (rsync error)", j.Src, j.Dst))
		return ExitEnv
	}

	// 「ディレクトリの属性・時刻だけの違い（.d…）」は中身とは無関係で、OS 側の
	// 都合で合わせられないことがある（exFAT のボリューム直下は 1970 年で固定）。
	// 合否はファイルの欠落・内容の違いだけで判定する。
	var sig []string
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, ".d") {
			continue
		}
		sig = append(sig, line)
	}

	if len(sig) > 0 {
		fmt.Fprintln(j.Err, "✗ 差分が見つかりました（コピー先に未転送/不一致のファイルがあります）:")
		shown := 0
		for _, l := range sig {
			if shown >= 20 {
				break
			}
			fmt.Fprintln(j.Out, l) // bash 版は標準出力へ
			shown++
		}
		if len(sig) > 20 {
			fmt.Fprintf(j.Out, "    …（他 %d 件）\n", len(sig)-20)
		}
		j.log(fmt.Sprintf("VERIFY diff %s vs %s", j.Src, j.Dst))
		return 1
	}

	// 集計は「参考情報」。転送先の余分なファイルは許容するため、
	// 数が一致しなくても失敗にしない（合否判定は上の rsync 差分で行う）。
	s := ScanDir(j.Src, j.AllExcludePatterns())
	d := ScanDir(j.Dst, j.AllExcludePatterns())
	extra := d.Files - s.Files

	fmt.Fprintf(j.Out, "    コピー元 %d ファイル / %s、転送先 %d ファイル / %s\n",
		s.Files, FmtBytes(s.Bytes), d.Files, FmtBytes(d.Bytes))
	switch {
	case extra > 0:
		fmt.Fprintf(j.Out, "==> 転送先には %d 件多いファイルがあります（削除しません）\n", extra)
	case extra < 0:
		fmt.Fprintf(j.Err, "! 転送先の方が %d 件少ないです（要確認）\n", -extra)
	}

	fmt.Fprintln(j.Out, "✓ 照合 OK（コピー元の全ファイルが転送先に存在）")
	j.log(fmt.Sprintf("VERIFY ok   %s vs %s (src %d files / dst %d files)", j.Src, j.Dst, s.Files, d.Files))
	return 0
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// RunJob は1件の処理（コピー → 照合）のオーケストレーション（bash 版 run_job）。
// mode: "copy" | "verify"
func RunJob(j *Job, verifyOnly bool, dryRun bool, doVerify bool) int {
	if verifyOnly {
		return VerifyOne(j)
	}

	if rc := CopyOne(j); rc != 0 {
		return rc
	}
	if dryRun {
		return 0
	}
	if doVerify {
		return VerifyOne(j)
	}
	return 0
}
