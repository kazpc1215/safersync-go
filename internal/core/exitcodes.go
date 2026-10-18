package core

// exitcodes.go - 終了コード（bash 版 bin/safersync と同じ意味）

const (
	ExitOK          = 0   // 成功
	ExitError       = 1   // エラー（引数の誤り、rsync の失敗など）
	ExitUnmounted   = 2   // コピー先のボリュームが未マウント
	ExitEnv         = 3   // 環境エラー（rsync 未検出、空き容量不足、差分・照合の実行不可など）
	ExitInterrupted = 130 // Ctrl-C で中断
)
