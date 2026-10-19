# safersync（Go 版）

rsync を安全に使うためのラッパー。**コピー → 照合** をワンストップで行い、進捗と残り時間を表示します。

[![CI](https://github.com/kazpc1215/safersync-go/actions/workflows/ci.yml/badge.svg)](https://github.com/kazpc1215/safersync-go/actions/workflows/ci.yml)

- **データを削除しません。** 転送先の余分なファイルも、転送元のファイルも消しません（`--delete` 非使用）
- **コピー先がコピー元の内側なら中止します。** `~/写真` の中へ `~/写真/backup` のように
  コピーすると実行のたびに入れ子で増え続けるため、開始前にエラーで止めます（同一パスも不可）
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
# 例: macOS (Apple Silicon)。<version> は入手したリリース番号（例: 1.5.2）に置き換える
tar -xzf safersync-<version>-darwin-arm64.tar.gz
mkdir -p ~/.local/bin
install safersync ~/.local/bin/safersync
```

チェックサム（`SHA256SUMS`）で整合性を確認してから配置することをすすめています。

### 方法 C: Homebrew

tap を公開済みの場合（`kazpc1215/homebrew-tap`）:

```sh
brew install kazpc1215/tap/safersync
```

formula は `dist/release/SHA256SUMS` から自動生成し、tarball の URL と sha256 を
リリースの実物に合わせます（生成は `packaging/homebrew/update-formula.sh`。手で編集しません）。

リリースを公開すると `Release` ワークフローが、そのリリースの `SHA256SUMS` から formula を
生成して本リポジトリの `main` へ自動反映し、リリースの添付ファイルにも含めます。
tap リポジトリ（`kazpc1215/homebrew-tap`）へも同じ内容を push します。ただし別リポジトリへ
push するための Secrets `TAP_GITHUB_TOKEN`（tap への push 権を持つ PAT）が必要で、未設定の
ときは自動反映をスキップします。その場合はリリース後に本リポジトリの
`packaging/homebrew/safersync.rb` を tap の `Formula/safersync.rb` へコピーしてください。

### 方法 D: ソースからビルド（Go 1.27 以降が入っている場合）

```sh
git clone https://github.com/kazpc1215/safersync-go.git
cd safersync-go && make install
```

または `go install` でも入手できます:

```sh
go install github.com/kazpc1215/safersync-go@latest
```

## 使い方

### CLI（TUI なし）

```sh
safersync ~/写真 /Volumes/HDD/backup              # コピー元 → コピー先を指定して実行
safersync --dry-run ~/写真 /Volumes/HDD/backup   # 転送内容のプレビューのみ
```

### リモートへのコピー（CLI のみ）

コピー元・コピー先には rsync のリモート表記も指定できます（`[user@]host:path`、
`rsync://host/module`）。SSH は rsync 側の設定（鍵・`~/.ssh/config`）に従います。

```sh
safersync ~/写真 backup@nas:/volume1/写真      # リモート先へ
safersync user@host:/data /Volumes/HDD/backup  # リモート元から
```

- リモートは**CLI 専用**です（対話ウィザードはボリューム選択のためローカルのみ）。
- リモートが絡む場合、**空き容量の確認**と**ファイル数の集計**（参考情報）は行いません。
  リモート先の進捗はバイト数ではなく**ファイル件数**で表示します。
- 照合（コピー後の自動照合）はリモートでも rsync の差分で行います。
- ローカルの名前に `:` を含む場合は、先頭に `./` を付けるとローカル扱いになります
  （例: `./撮影:2019`）。これは rsync と同じ規則です。

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

## 環境変数

| 変数 | 説明 |
|---|---|
| `SAFERSYNC_MOUNT_ROOT` | 外付けボリュームの基点（`--mount-root` と同じ） |
| `SAFERSYNC_LOG_DIR` | ログの場所（既定: macOS は `~/Library/Logs/safersync`） |
| `SAFERSYNC_THEME` | TUI の配色（`dark`（既定）/ `light` / `high-contrast`） |
| `NO_COLOR` | 設定すると色を使わない（TUI・CLI 共通） |

ログは 5 MiB を超えると `safersync.log.1` に1世代だけ退避します（無限に育ちません）。

## 開発

```sh
make test      # go test ./...
make vet fmt   # 静的検査 + 整形
make build     # dist/ に全ターゲット（darwin/amd64, darwin/arm64, linux/amd64, linux/arm64）の単一バイナリ
make release   # リリース物（dist/release/*.tar.gz + SHA256SUMS）
make formula   # release の SHA256SUMS から Homebrew formula を生成（tap へコピーする用）
```

変更点は [`CHANGELOG.md`](CHANGELOG.md) にまとめています。リリースのメモは GitHub Actions が
自動生成します。

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
- `install.sh` は [`scripts/test-install.sh`](scripts/test-install.sh) で検証します。その場で
  現在のプラットフォーム向けの tarball と `SHA256SUMS` を作り、`file://` のミラー経由で
  導入と `--version` 確認、不正バージョンの拒否までを通します（ネットワーク不要）。
  CI の `go` ジョブで実行し、`shellcheck` もかけます（Linux ランナー）。

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
