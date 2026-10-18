package core

// sync.go - rsync 実行・差分収集・除外制御（bash 版 lib/core/sync.sh の Go 移植）

import (
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

	TotalFiles int // この Job でコピーした累計（成功時のみ加算）
	TotalBytes int64
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

// ---------------------------------------------------------------------------
// 差分収集（bash 版 collect_diff / plan_changes）
// ---------------------------------------------------------------------------

// CollectDiff は rsync の dry-run を行い、--out-format の結果を返す（空なら差分なし）。
// bash 版と同じく、呼び出し時の失敗（rsync 無し等）は黙って空文字にする。
func (j *Job) CollectDiff(outFormat string) string {
	args := []string{"-n", "--out-format=" + outFormat}
	args = append(args, j.rsyncMetaOpts()...)
	args = append(args, j.excludeArgs()...)
	if j.Checksum {
		args = append(args, "--checksum")
	}
	args = append(args, j.Src+"/", j.Dst+"/")
	out, _ := exec.Command("rsync", args...).Output()
	return string(out)
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
}

// PlanChanges は転送対象のファイルだけを集める。
// ディレクトリや属性（時刻）だけの変化は数えない（bash 版と同じ）。
func (j *Job) PlanChanges() Plan {
	raw := j.CollectDiff("%i %l %n")
	var plan Plan
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
