# Changelog

本プロジェクトの主な変更点を記録します。書式は [Keep a Changelog](https://keepachangelog.com/ja/1.1.0/)、
バージョニングは [Semantic Versioning](https://semver.org/lang/ja/) に従います。

## [Unreleased]

## [1.4.2] - 2026-10-19

### 追加

- TUI のテスト網羅率を 42.6% → 74.1% に向上（ステージ遷移・フォルダ作成・履歴・ボリューム表示・
  マウス・ヘルパ・引数/終了コード契約・`DfHuman`・走査キャッシュ等）

### 変更

- `--exclude` / `--mount-root` の値が無い・空の場合も `--from`/`--to` と同様にエラーにする（黙殺しない）
- `--help` の終了コード表を実装（`ExitEnv`）に合わせた
- `DfHuman` の列位置決め打ちを廃止し、デバイス名の空白・GNU df の折り返しにも対応
- コピー元の走査結果をキャッシュし、進捗の分母と照合の集計で同じ木を何度も走査しない
- 転送進捗の走査を非同期化し、大きなツリーでも UI/シグナル処理をブロックしない
- `--verify-only` が `--dry-run` と併用された場合は照合を優先することを help に明記

### 修正

- Makefile の `make help` が `.PHONY` 等を表示しないようにした
- staticcheck のピン（v0.8.2）が現行リリースであることをコメントで明記

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

[Unreleased]: https://github.com/kazpc1215/safersync-go/compare/v1.4.2...HEAD
[1.4.2]: https://github.com/kazpc1215/safersync-go/compare/v1.4.1...v1.4.2
[1.4.1]: https://github.com/kazpc1215/safersync-go/compare/v1.4.0...v1.4.1
