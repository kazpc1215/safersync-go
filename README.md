# safersync（Go 版）

`rsync` の安全なラッパー（コピー → 照合）の Go/Bubble Tea 移植版。
出力文言・終了コードは bash 版（~/safersync）と同じ契約を保つ。

## 現在の状態

- [x] ステップ 1: スケルトン（`--version` / `--help` / 引数解析・エラー文言）
- [ ] ステップ 2: core（マウント判定・空き容量・rsync 呼び出し・照合）の移植
- [ ] ステップ 3: TUI（Bubble Tea）の移植

## 開発

```sh
make test     # go test ./...
make fmt vet  # 整形 + 静的検査
make build    # 全ターゲット（darwin/amd64+arm64, linux/amd64+arm64）を dist/ へ
```

実行は `go run . --version` のように、または `go build -o safersync .` で生成した
単一バイナリを使う。ランタイムのインストール不要という bash 版の売りを維持する。

## テスト契約

- bash 版の `test/safersync.bats` が CLI の受け入れテストになる（`--version` は
  `safersync v1.2.0`、`--help` の必須オプション一覧、終了コード 0/1/2/3/130 など）。
- Go 側のユニットテストは `internal/cli/cli_test.go` ほかに随時追加する。
