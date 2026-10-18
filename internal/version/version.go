// Package version は safersync のバージョンを管理する。
// bash 版との互換性を保つため、Go 版は v1.2.0 から開始する。
//
// ビルド時に make release 経由で -ldflags "-X ...=version.Version" により
// 上書きできる（CI のリリースビルド用）。
package version

var Version = "1.3.0"
