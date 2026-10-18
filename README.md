# safersync（Go 版）

rsync を安全に使うためのラッパー。**コピー → 照合** をワンストップで行い、進捗と残り時間を表示します。

[![CI](https://github.com/kazpc1215/safersync-go/actions/workflows/ci.yml/badge.svg)](https://github.com/kazpc1215/safersync-go/actions/workflows/ci.yml)

- **データを削除しません。** 転送先の余分なファイルも、転送元のファイルも消しません（`--delete` 非使用）
- **設定ファイルはありません。** コピー元・コピー先・除外などはすべてコマンドライン引数で指定します
- **ランタイム不要。** Go で単一バイナリにコンパイルされているため、Homebrew や Python なしでも動きます
  （内部で標準搭載の `rsync` を呼び出します）
- **対話ウィザード（TUI）付き。** 引数なしで対話端末から起動すると、ボリューム選択から完了レポートまで案内します

bash 版（[kazpc1215/safersync](https://github.com/kazpc1215/safersync)）のフル移植で、CLI の出力文言・終了コードは bash 版と同じ契約を保ちます。

## インストール

特別なツールやランタイムのインストールは不要です。macOS（Intel / Apple Silicon）と Linux（x86_64 / ARM64）の
静的リンク単一バイナリとして提供されます。

### 方法 A: インストーラ（推奨・1 行）

```sh
curl -fsSL https://raw.githubusercontent.com/kazpc1215/safersync-go/main/install.sh | sh
```

`~/.local/bin/safersync` に配置し、PATH が通っていない場合は追加を案内します。
ダウンロードした tarball はリリースの `SHA256SUMS` と照合し、一致しない場合・
チェックサムを取得できない場合はインストールを中止します。
`safersync: command not found` が出る場合は、シェル設定（`~/.zshrc` など）に:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

を追記して `source ~/.zshrc` してください。

### 方法 B: リリースから手動で配置

[Releases](https://github.com/kazpc1215/safersync-go/releases) から環境に合う tarball を
ダウンロードして解凍し、`safersync` を PATH の通った場所へ置きます。

```sh
# 例: macOS (Apple Silicon)
tar -xzf safersync-1.4.0-darwin-arm64.tar.gz
mkdir -p ~/.local/bin
install safersync ~/.local/bin/safersync
```

チェックサム（`SHA256SUMS`）で整合性を確認してから配置することをすすめています。

### 方法 C: Homebrew

tap を公開済みの場合（`kazpc1215/homebrew-tap`）:

```sh
brew install kazpc1215/tap/safersync
```

formula は本リポジトリの `packaging/homebrew/safersync.rb` を tap 側の `Formula/` に
コピーして利用します（tarball の URL と sha256 はリリースの `SHA256SUMS` から採用します）。

### 方法 D: ソースからビルド（Go 1.27 以降が入っている場合）

```sh
git clone https://github.com/kazpc1215/safersync-go.git
cd safersync-go && make install
```

または `go install` でも入手できます:

```sh
go install github.com/kazpc1215/safersync@latest
```

## 使い方

### CLI（TUI なし）

```sh
safersync ~/写真 /Volumes/HDD/backup              # コピー元 → コピー先を指定して実行
safersync --dry-run ~/写真 /Volumes/HDD/backup   # 転送内容のプレビューのみ
```

### TUI（対話ウィザード）

```sh
safersync          # 引数なし・対話端末ならウィザード起動
safersync -i       # 引数ありでも明示的に TUI で起動
safersync --tui ~/写真 /Volumes/HDD/backup   # コピー元/先をプリフィルして起動
```

ウィザードの流れ: **コピー元の選択 → コピー先の選択 → 同期オプション → 転送内容の最終確認 →
転送ダッシュボード → 完了レポート**

- コピー元・先は下位フォルダに移動して選べます（存在しないフォルダはその場で作成できます）
- プレビューで空き容量不足や未マウントを検知したら開始をブロックします
- 転送中の `Ctrl-C` は安全に中断します（rsync を確実に停止し、次回は続きから再開できます）

> 詳しい使い方・出力の読み方・終了コードなどは bash 版 README と共通です:
> <https://github.com/kazpc1215/safersync>

## 終了コード

| コード | 意味 |
|---:|---|
| 0 | 成功 |
| 1 | エラー（引数の誤り、コピー元が無い、rsync の失敗・照合不一致など） |
| 2 | コピー先のボリュームが未マウント |
| 3 | 環境エラー（rsync 未検出、空き容量不足、差分・照合の実行不可など） |
| 130 | `Ctrl-C` で中断 |

## 開発

```sh
make test      # go test ./...
make vet fmt   # 静的検査 + 整形
make build     # dist/ に全ターゲット（darwin/amd64, darwin/arm64, linux/amd64, linux/arm64）の単一バイナリ
make release   # リリース物（dist/release/*.tar.gz + SHA256SUMS）
```

`make release` の tarball はいずれも静的リンク（`CGO_ENABLED=0`）で、バージョンは
`make VERSION=1.2.1` のように上書きできます。

## テスト契約

- bash 版の [`test/safersync.bats`](https://github.com/kazpc1215/safersync/tree/main/test) が
  CLI の受け入れテストとして **回帰テスト** になっています。bash 版リポジトリを一式
  （`lib/` と `test/`）取得し、`bin/safersync` だけ Go バイナリへ差し替えて bats を実行します
  （CI の `bats-regression` ジョブと同じ手順）。
  - 検証済み: コピー / 照合 / dry-run / 除外 / 空き容量 / 未マウント判定 / `--status` /
    ログ配置 など。bash 版の bats テスト 47 件（shellcheck は bash 版 `lib/` の静的検査）が
    Go バイナリ差し替えで**すべて合格**（CI の `bats-regression` ジョブで毎回実行）。
- Go 側のユニットテストは [`internal/cli/cli_test.go`](internal/cli/cli_test.go)、
  [`internal/core/core_test.go`](internal/core/core_test.go)、
  [`internal/tui/tui_test.go`](internal/tui/tui_test.go)。

## ファイル構成

bash 版の「core（純ロジック）/ tui（画面）」分離を踏襲しています:

```
main.go                  エントリポイント
internal/cli/            CLI の出力・引数解析・終了コード（bash 版 bin/safersync 相当）
internal/core/           マウント判定・空き容量・rsync 実行・照合（bash 版 lib/core 相当）
internal/tui/            対話ウィザード（Bubble Tea。bash 版 lib/tui 相当）
internal/version/        バージョン（リリースビルドで -ldflags により上書き可）
packaging/homebrew/      Homebrew tap 用 formula
install.sh               リリース単一バイナリのインストーラ（curl | sh 用）
```

## 移行について（bash 版から）

- bash 版リポジトリ（kazpc1215/safersync）は機能追加を停止し、この Go 版へ統合しました
- コマンド体系・出力・終了コードは bash 版と同一のため、既存のスクリプトや bats テストは
  書き換えずにそのまま使えます（47 件すべてが合格する回帰テストがこの互換性の証拠です）
- bash 版を引き続き使いたい場合は `~/safersync/bin/safersync` を直接実行するか、シンボリックリンクで使えます
