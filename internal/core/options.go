// Package core は画面出力を持たない純ロジック（引数解析の結果、盤面判定、
// rsync 呼び出し、照合）を格納する。bash 版の lib/core に対応する。
package core

// JobSpec は複数ジョブ実行（--job）用の 1 組。
type JobSpec struct {
	From string
	To   string
}

// Options はコマンドライン引数の解析結果。
// bash 版 bin/safersync のオプション一覧と同じ項目を持つ。
type Options struct {
	// 表示系
	ShowVersion bool
	ShowHelp    bool
	ShowStatus  bool

	// TUI
	TUI bool

	// コピー元・コピー先（位置引数 / --from / --to のどちらでも指定可）
	From string
	To   string

	// 複数ジョブ（--job SRC DST を繰り返し指定）
	Jobs []JobSpec

	// オプション
	MountRoot        string   // 外付けボリュームの基点（既定 /Volumes、--mount-root・環境変数で上書き）
	LogDir           string   // ログの場所（SAFERSYNC_LOG_DIR。未指定なら OS 既定）
	Excludes         []string // 追加除外パターン（--exclude、複数可）
	FAT              bool     // exFAT/FAT（既定）: 属性差を比較しない
	APFS             bool     // APFS/HFS+: 属性も保持・比較する
	DryRun           bool     // -n / --dry-run
	VerifyOnly       bool     // --verify-only
	NoVerify         bool     // --no-verify
	Checksum         bool     // -c / --checksum
	IgnoreSpaceCheck bool     // --ignore-space-check
	Stats            bool     // -v / --stats
	Progress         bool     // --progress
	NoProgress       bool     // --no-progress
	NoCaffeinate     bool     // --no-caffeinate
}

// NewOptions は既定値で初期化した Options を返す。
// bash 版: コピー先は FAT 想定（既定）、進捗は端末なら ON。
func NewOptions() *Options {
	return &Options{FAT: true}
}
