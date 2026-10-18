package core

// sync.go - rsync 実行・差分収集・除外制御（bash 版 lib/core/sync.sh の Go 移植）

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// DefaultExcludes は常に適用する除外パターン（bash 版 DEFAULT_EXCLUDES と同じ）。
var DefaultExcludes = []string{
	".DS_Store",
	"._*",
	".Spotlight-V100",
	".fseventsd",
	".Trashes",
	".TemporaryItems",
	".AppleDouble",
}

// Job は「コピー → 照合」の1回の処理。CLI・（後の）TUI の両方から使う。
// 画面出力は Out / Err に流すので、core 自体は表示方法を知らない。
type Job struct {
	Src, Dst     string
	Excludes     []string // 追加除外（--exclude、複数可）
	FAT          bool     // true: -rlt --modify-window=2（exFAT 既定）/ false: -a
	Checksum     bool     // -c
	IgnoreSpace  bool     // --ignore-space-check
	Verbose      bool     // -v（rsync --stats）
	ShowProgress bool     // 進捗表示するか（端末のときだけ true にして渡す）
	ListFiles    bool     // TUI 用: 転送中ファイル名を --out-format で出力する
	NoCaffeinate bool
	DryRun       bool

	Log func(msg string) // ログ hook（nil 可。core.LogMsg を渡す）
	Out io.Writer        // 通常出力（stdout）
	Err io.Writer        // 警告・エラー（stderr、tty 判定は CLI 側で）

	// Ctx は差分・照合用の rsync をキャンセル可能にする（TUI の Ctrl-C 用）。
	// nil のときは従来どおり実行する。
	Ctx context.Context

	TotalFiles int // この Job でコピーした累計（成功時のみ加算）
	TotalBytes int64

	// srcScanCache はコピー元の走査結果のキャッシュ。コピー元はこの処理の間は
	// 変化しないため、進捗の分母（runWithProgress）と照合の集計（VerifyOne）で
	// 同じ木を何度も走査しないよう共有する。nil なら未走査。
	// 同時アクセスは想定しない（各呼び出しは同じ goroutine から順に行う）。
	srcScanCache *ScanResult
}

func (j *Job) log(msg string) {
	if j.Log != nil {
		j.Log(msg)
	}
}

// ---------------------------------------------------------------------------
// オプション組み立て（bash 版 build_rsync_opts）
// ---------------------------------------------------------------------------

// rsyncMetaOpts は rsync に渡すメタ保持オプション。
//
//	FAT モード（既定）:
//	  -rlt: 再帰・シンボリックリンク・更新時刻を保持（-p/-o/-g は付けない）
//	  --modify-window=2: exFAT の時刻丸めによる誤差を許容
//	-a（--apfs）: パーミッション・所有権なども保存する
func (j *Job) rsyncMetaOpts() []string {
	if j.FAT {
		return []string{"-rlt", "--modify-window=2"}
	}
	return []string{"-a"}
}

func (j *Job) excludeArgs() []string {
	pats := append(append([]string{}, DefaultExcludes...), j.Excludes...)
	out := make([]string, 0, len(pats)+2)
	for _, e := range pats {
		if e != "" {
			out = append(out, "--exclude="+e)
		}
	}
	return out
}

// AllExcludePatterns は走査に使う除外（転送時と同じもの）。
func (j *Job) AllExcludePatterns() []string {
	return append(append([]string{}, DefaultExcludes...), j.Excludes...)
}

// SrcScan はコピー元を走査した結果を返す。初回だけ実際に走査し、以降は
// キャッシュを返す。コピー元は処理中に変化しない前提なので、進捗の分母と
// 照合の集計で走査を共有でき、大きなツリーでの二重走査を避けられる。
func (j *Job) SrcScan() ScanResult {
	if j.srcScanCache == nil {
		r := ScanDir(j.Src, j.AllExcludePatterns())
		j.srcScanCache = &r
	}
	return *j.srcScanCache
}

// SetSrcScan は走査済みのコピー元の結果を設定し、再走査を省く。
// TUI はプレビューで走査した結果を転送・照合の Job へ引き継ぐために使う。
func (j *Job) SetSrcScan(r ScanResult) {
	j.srcScanCache = &r
}

// ---------------------------------------------------------------------------
// 差分収集（bash 版 collect_diff / plan_changes）
// ---------------------------------------------------------------------------

// RsyncAvailable は rsync が PATH から実行できるかを返す。
// rsync が無いまま処理を進めると「差分なし」「照合 OK」と誤って成功を報告して
// しまうため、実行前にここで確認する（終了コード 3）。
func RsyncAvailable() bool {
	_, err := exec.LookPath("rsync")
	return err == nil
}

// rsyncExitVanished は「走査中にファイルが消えた」(rc=24)。
// 撮影中の SD カードなどでは dry-run でも起こり得るため、これは成功扱いにする。
const rsyncExitVanished = 24

// newDiffCmd は dry-run 用の rsync コマンドを作る。j.Ctx があればキャンセル対応
// （TUI の Ctrl-C でスキャン中の rsync を止める）。
func newDiffCmd(j *Job, args []string) *exec.Cmd {
	if j.Ctx != nil {
		return exec.CommandContext(j.Ctx, "rsync", args...)
	}
	return exec.Command("rsync", args...)
}

// CollectDiff は rsync の dry-run を行い、--out-format の結果を返す（空なら差分なし）。
// rsync を実行できないとき（未インストール・リモートへ到達不能など）はエラーを返す。
// 「差分なし」と「確認できなかった」を取り違えると、照合 OK の誤報告につながる。
func (j *Job) CollectDiff(outFormat string) (string, error) {
	args := []string{"-n", "--out-format=" + outFormat}
	args = append(args, j.rsyncMetaOpts()...)
	args = append(args, j.excludeArgs()...)
	if j.Checksum {
		args = append(args, "--checksum")
	}
	args = append(args, j.Src+"/", j.Dst+"/")
	out, err := newDiffCmd(j, args).Output()
	if err != nil {
		// 中断（TUI の Ctrl-C など）はエラー表示せず、そのまま呼び出し側に伝える
		if j.Ctx != nil && j.Ctx.Err() != nil {
			return string(out), j.Ctx.Err()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if ee.ExitCode() == rsyncExitVanished {
				return string(out), nil // 消えたファイルは差分に含めない
			}
			if msg := firstLine(string(ee.Stderr)); msg != "" {
				return "", fmt.Errorf("rsync を実行できませんでした: %s", msg)
			}
			return "", fmt.Errorf("rsync がエラー終了しました (rc=%d)", ee.ExitCode())
		}
		return "", fmt.Errorf("rsync を実行できませんでした: %w", err)
	}
	return string(out), nil
}

// firstLine は rsync のエラー出力（stderr）から最初の空でない行を取り出す。
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// Plan はコピーが必要なファイルの「件数・合計バイト数・一覧」。
type Plan struct {
	Files int
	Bytes int64
	Lines []string // "item name" 形式（dry-run 表示用）

	// 差分詳細（タブ表示用）
	Transfer []string // 転送されるファイル（>f...）
	Skipped  []string // スキップ/属性のみの変化（.f... / .d...）
	Other    []string // その他の差分

	// Err は差分を取得できなかった場合（rsync 不在・到達不能など）に設定される。
	// このとき Files 等は 0 のままで、「差分なし」と区別する。
	Err error
}

// PlanChanges は転送対象のファイルだけを集める。
// ディレクトリや属性（時刻）だけの変化は数えない（bash 版と同じ）。
func (j *Job) PlanChanges() Plan {
	var plan Plan
	raw, err := j.CollectDiff("%i %l %n")
	if err != nil {
		plan.Err = err
		return plan
	}
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		item, size, name, ok := parseDiffLine(line)
		if !ok {
			continue
		}
		entry := item + " " + name
		switch {
		case strings.HasPrefix(item, ">f"):
			plan.Files++
			plan.Bytes += size
			plan.Lines = append(plan.Lines, entry)
			plan.Transfer = append(plan.Transfer, entry)
		case strings.HasPrefix(item, "."):
			plan.Skipped = append(plan.Skipped, entry)
		default:
			plan.Other = append(plan.Other, entry)
		}
	}
	return plan
}

// parseDiffLine は "item size name..." （name は空白を含み得る）を分解する。
func parseDiffLine(line string) (item string, size int64, name string, ok bool) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return "", 0, "", false
	}
	item = parts[0]
	name = parts[2]
	if _, err := fmt.Sscanf(parts[1], "%d", &size); err != nil {
		size = 0
	}
	return item, size, name, true
}
