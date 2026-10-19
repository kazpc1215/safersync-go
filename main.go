// safersync - rsync の安全なラッパー（コピー → 照合）
//
// bash 版（~/safersync）の Go 移植。CLI の出力と終了コードは bash 版と同じ契約を保つ。
package main

import (
	"os"

	"github.com/kazpc1215/safersync-go/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
