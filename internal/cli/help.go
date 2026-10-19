package cli

import (
	"fmt"
	"io"

	"github.com/kazpc1215/safersync/internal/version"
)

const helpText = `safersync v%s - rsync の安全なラッパー（コピー → 照合）

  このツールはデータを削除しません。
  転送先の余分なファイルも、転送元のファイルも消しません。
  設定ファイルは使いません（すべて引数で指定します）。

使い方
  safersync [オプション] <コピー元> <コピー先>

  コピー元はフォルダのパス、コピー先は保存先フォルダのパスを指定します。
  コピー元フォルダの中身が、コピー先フォルダの中へ入ります（追加のみ）。
  コピー先フォルダが無ければ作られます。
  末尾の "/" は付けても付けなくても同じ意味になります。

  コピー先がコピー元の内側になる指定（例: ~/写真 → ~/写真/backup）は、
  実行のたびにコピーが入れ子になって増え続けるため、開始前にエラーになります。
  コピー元とコピー先を同じにすることもできません。
  コピー元の外（別フォルダや別ボリューム）を指定してください。

  コピー元・コピー先にはリモート（user@host:/path、rsync://host/mod）も指定できます。
  ただしリモートは CLI のみです（対話ウィザードはローカルのみ）。
  リモートが絡む場合は、空き容量の確認とファイル数の集計を行いません
  （リモート先の進捗はファイル件数で表示します）。

  引数なしで起動する（safersync）と、対話ウィザード（TUI）が開き、
  矢印キーだけでコピー元・コピー先を選べます。-i でも起動します。

例
  safersync ~/写真 /Volumes/HDD/写真
      # → HDD の 写真フォルダへコピー＆照合（無ければ作られる）
  safersync ~/書類 /Volumes/HDD/backup/書類
      # → HDD 内の下位フォルダも自由に指定できる
  safersync ~/写真 /Volumes/HDD/写真 --verify-only   # 照合のみ
  safersync ~/写真 /Volumes/HDD/写真 -n              # dry-run
  safersync ~/書類 /Volumes/HDD/書類 --exclude '*.tmp'    # 追加で除外
  safersync --status                                              # ボリュームの状態

オプション
  -i, --tui            対話ウィザード（TUI）を起動する。引数なしでも起動する
      --mount-root DIR 外付けボリュームの基点（既定: /Volumes。例: /media/ユーザー名）
  -t, --to DIR         コピー先（位置引数と同じ。順序は自由）
  -f, --from DIR       コピー元（位置引数と同じ。順序は自由）
      --job SRC DST    複数ジョブを順次実行（繰り返し指定可）
  -x, --exclude PAT    追加で除外するパターン（複数可。例: '*.tmp'）
      --fat            コピー先が exFAT/FAT（既定。属性差を比較しない）
      --apfs           コピー先が APFS/HFS+（属性も保持・比較する）
  -n, --dry-run        実際にはコピーせず、対象を確認する
      --verify-only    コピーせず照合のみ行う（--dry-run より優先される）
      --no-verify      コピー後の自動照合を行わない
  -c, --checksum       内容のハッシュで照合（厳密だが遅い）
      --ignore-space-check  空き容量が足りなくても中止せず続行する
  -v, --stats          rsync の詳細な統計を表示する（既定は非表示）
      --progress       進捗と残り時間（ETA）を表示（端末なら既定でON）
      --no-progress    進捗表示をしない（パイプやログ向け）
      --no-caffeinate  スリープ防止（caffeinate）を使わない
      --status         マウント中のボリュームと空き容量を表示
  -h, --help           このヘルプ
  -V, --version        バージョン表示

終了コード
  0 成功 / 1 エラー（引数の誤り・コピー元が無い・照合不一致など）
  2 コピー先が未マウント / 3 空き容量不足（ほかに rsync 未検出・差分/照合の実行不可など） / 130 中断

環境変数
  SAFERSYNC_MOUNT_ROOT  外付けボリュームの基点（--mount-root と同じ）
  SAFERSYNC_LOG_DIR     ログの場所（既定: macOS は ~/Library/Logs/safersync）
  SAFERSYNC_THEME       TUI の配色（dark 既定 / light / high-contrast）
  NO_COLOR              設定すると色を使わない（TUI・CLI 共通）

ログ: %s
`

func printHelp(w io.Writer, logFile string) {
	fmt.Fprintf(w, helpText, version.Version, logFile)
}
