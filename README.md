# safersync（Go 版）

`rsync` の安全なラッパー（コピー → 照合）の Go/Bubble Tea 移植版。
出力文言・終了コードは bash 版（~/safersync）と同じ契約を保つ。

## 現在の状態

- [x] ステップ 1: スケルトン（`--version` / `--help` / 引数解析・エラー文言）
- [x] ステップ 2: core（マウント判定・空き容量・rsync 呼び出し・照合）の移植
- [x] ステップ 3: TUI（Bubble Tea）の移植
  - `safersync` / `safersync --tui` / `-i` でウィザード起動（非対話端末では起動しない）
  - ボリューム選択 → フォルダ階層移動（新規フォルダ作成・手動入力）→
    除外/照合オプション → 差分プレビュー → 転送ダッシュボード（進捗・ETA）→
    完了レポート。Ctrl-C で安全中断（終了コード 130）
  - 依存は bubbletea + lipgloss のみ（単一バイナリを維持）

## 開発

```sh
make test     # go test ./...
make fmt vet  # 整形 + 静的検査
make build    # 全ターゲット（darwin/amd64+arm64, linux/amd64+arm64）を dist/ へ
```

実行は `go run . --version` のように、または `go build -o safersync .` で生成した
単一バイナリを使う。ランタイムのインストール不要という bash 版の売りを維持する。

## テスト契約

- bash 版の `test/safersync.bats` が CLI の受け入れテストになる。実際に
  バイナリを差し替えて検証済み（コピー / 照合 / dry-run / 除外 / 空き容量 /
  未マウント判定 / `--status` / ログ配置 など **46/47 が合格**）。
  - 唯一の不合格は shellcheck テスト（bash ソース専用）。Go 版では
    `go vet ./...` + `go test ./...` が同等のゲート。
- Go 側のユニットテストは `internal/cli/cli_test.go` ほかに随時追加する。
