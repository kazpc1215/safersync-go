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
var Version = strings.TrimSpace(versionFile)
