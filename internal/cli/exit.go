// Package cli はコマンドラインの引数解析・ふるまいを担う。
// bash 版 bin/safersync と同じ出力文言・終了コードを保つ。
package cli

// 終了コード（bash 版と同じ）
const (
	ExitOK          = 0   // 成功
	ExitError       = 1   // エラー（引数の誤り、rsync の失敗など）
	ExitUnmounted   = 2   // コピー先のボリュームが未マウント
	ExitNoSpace     = 3   // 空き容量が足りない
	ExitInterrupted = 130 // Ctrl-C で中断
)
