# Changelog

本プロジェクトの主な変更点を記録します。書式は [Keep a Changelog](https://keepachangelog.com/ja/1.1.0/)、
バージョニングは [Semantic Versioning](https://semver.org/lang/ja/) に従います。

## [Unreleased]

## [1.4.1] - 2026-10-19

### 変更

- Homebrew formula をリリースの `SHA256SUMS` から生成するようにしました（`make formula`）。
  生成物はリリースの添付ファイルに含め、タグ公開後にリポジトリの `packaging/homebrew/` へ自動同期します
- 引数検証のテストを、テスト専用の複製ではなく実エントリポイント（`Run`）経由で検証するようにしました

### 修正

- **コピー先がコピー元の内側にある場合（およびコピー元と同一パス）を、開始前にエラーで中止**するようにしました。
  実行のたびにコピーが入れ子になって増え続ける事故を防ぎます（bash 版は許可していたため挙動変更）
- TUI の転送進捗ゴルーチンがモデルの状態を読み出していたデータ競合を修正しました
- Linux でスペース等がエスケープ（`\040` など）されたマウントポイントを正しく検出できるようにしました

## [1.4.0] 以前

個々の変更はリリースノートと git 履歴を参照してください:
<https://github.com/kazpc1215/safersync-go/releases>

[Unreleased]: https://github.com/kazpc1215/safersync-go/compare/v1.4.1...HEAD
[1.4.1]: https://github.com/kazpc1215/safersync-go/compare/v1.4.0...v1.4.1
