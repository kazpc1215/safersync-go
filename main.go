// safersync - rsync の安全なラッパー（コピー → 照合）
//
// bash 版（~/safersync）の Go 移植。CLI の出力と終了コードは bash 版と同じ契約を保つ。
package main

import (
	"io"
	"os"

	"github.com/kazpc1215/safersync-go/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run は main の実体。入出力を注入できるようにしてテスト可能にする
// （main を直接呼ぶと os.Exit でテストプロセスが終了してしまうため）。
func run(args []string, stdout, stderr io.Writer) int {
	return cli.Run(args, stdout, stderr)
}
