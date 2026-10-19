// Package version は safersync のバージョンを管理する。
//
// バージョンの単一ソースは同じディレクトリの VERSION ファイル。Makefile と
// リリースワークフローもこのファイルを読む（以前は Makefile と Go の定数の
// 二重管理で、更新漏れが起きやすかった）。
//
// リリースビルドでは -ldflags "-X ...=version.Version" による上書きもできる。
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version は VERSION ファイルの内容（前後の空白は除去）。
//
// 初期化を init 内で行うのは、-ldflags "-X ...=version.Version=..." による
// 上書きを保持するため。パッケージ変数の初期化式で代入すると、リンカが設定した
// 値を起動時の init が上書きしてしまう。
var Version string

func init() {
	if Version == "" {
		Version = strings.TrimSpace(versionFile)
	}
}
