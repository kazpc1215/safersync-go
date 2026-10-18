package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kazpc1215/safersync/internal/core"
	"github.com/kazpc1215/safersync/internal/version"
)

// Run は safersync の本体。引数解析から表示・動作のふりわけまで行い、
// 終了コードを返す。
func Run(argv []string, stdout, stderr io.Writer) int {
	opts, err := Parse(argv)
	if err != nil {
		fmt.Fprintf(stderr, "✗ %s\n", err)
		return ExitError
	}

	switch {
	case opts.ShowVersion:
		fmt.Fprintf(stdout, "safersync v%s\n", version.Version)
		return ExitOK
	case opts.ShowHelp:
		printHelp(stdout)
		return ExitOK
	}

	// 共通の前処理: 環境変数・マウント基点の決定
	applyEnv(opts)

	// ここから先はステップ 2 以降で実装する処理
	if opts.ShowStatus {
		return notImplemented(stderr, "--status（ボリューム表示）")
	}
	if opts.TUI || (opts.From == "" && opts.To == "") {
		return notImplemented(stderr, "対話ウィザード（TUI）")
	}
	if err := validate(opts); err != nil {
		fmt.Fprintf(stderr, "✗ %s\n", err)
		return ExitError
	}
	return notImplemented(stderr, "コピー処理")
}

// applyEnv は環境変数をオプションへ反映する。
func applyEnv(opts *core.Options) {
	if opts.MountRoot == "" {
		if v := os.Getenv("SAFERSYNC_MOUNT_ROOT"); v != "" {
			opts.MountRoot = v
		}
	}
	if opts.LogDir == "" {
		if v := os.Getenv("SAFERSYNC_LOG_DIR"); v != "" {
			opts.LogDir = v
		}
	}
}

// validate は引数の整合性を確認する。bash 版と同じ文言でエラーを返す。
func validate(opts *core.Options) error {
	if opts.From == "" {
		return fmt.Errorf("コピー元を指定してください")
	}
	if opts.To == "" {
		return fmt.Errorf("コピー先を指定してください")
	}
	return nil
}

// notImplemented は未実装の機能に一時的な案内を出す。
func notImplemented(stderr io.Writer, what string) int {
	fmt.Fprintf(stderr, "✗ この機能はまだ移植中です: %s\n（bash 版 ~/safersync/bin/safersync をご利用ください）\n", what)
	return ExitError
}

// logPath はデフォルトのログパスを返す。
func logPath(opts *core.Options) string {
	if strings.TrimSpace(opts.LogDir) == "" {
		return ""
	}
	return filepath.Join(opts.LogDir, "safersync.log")
}
