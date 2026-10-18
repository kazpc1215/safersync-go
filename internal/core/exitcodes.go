package core

// exitcodes.go - 終了コード（bash 版 bin/safersync と同じ意味）

const (
	ExitOK          = 0   // 成功
	ExitError       = 1   // エラー（引数の誤り、rsync の失敗など）
	ExitUnmounted   = 2   // コピー先のボリュームが未マウント
	ExitNoSpace     = 3   // 空き容量が足りない
	ExitInterrupted = 130 // Ctrl-C で中断
)
