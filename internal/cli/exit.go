package cli

// 終了コード（core パッケージに定義。bash 版と同じ）
import "github.com/kazpc1215/safersync/internal/core"

const (
	ExitOK          = core.ExitOK
	ExitError       = core.ExitError
	ExitUnmounted   = core.ExitUnmounted
	ExitNoSpace     = core.ExitNoSpace
	ExitInterrupted = core.ExitInterrupted
)
