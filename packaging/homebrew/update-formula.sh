#!/bin/sh
#
# update-formula.sh - dist/release/SHA256SUMS から Homebrew formula を生成する
#
#   dist/ の tarball は gzip が mtime を埋め込むためバイト再現性がなく、
#   formula の sha256 は「実際に公開したリリースの値」でなければ検証に通らない。
#   そのため手で編集せず、`make release` の直後に実行して生成する。
#
#   使い方:
#     packaging/homebrew/update-formula.sh <SHA256SUMS> <VERSION> [出力先]
#   例:
#     packaging/homebrew/update-formula.sh dist/release/SHA256SUMS 1.4.0
#
#   生成後、tap リポジトリ（kazpc1215/homebrew-tap）の Formula/safersync.rb へ
#   コピーして使う。
set -eu

sums=${1:?SHA256SUMS のパスを指定してください}
ver=${2:?バージョンを指定してください（例: 1.4.0）}
out=${3:-packaging/homebrew/safersync.rb}

# get <os>-<arch> は SHA256SUMS から該当 tarball のハッシュを返す
get() {
  awk -v f="safersync-${ver}-$1.tar.gz" '$2 == f || $2 == "*" f { print $1; exit }' "$sums"
}

darwin_arm=$(get darwin-arm64)
darwin_amd=$(get darwin-amd64)
linux_arm=$(get linux-arm64)
linux_amd=$(get linux-amd64)

for v in "$darwin_arm" "$darwin_amd" "$linux_arm" "$linux_amd"; do
  if [ -z "$v" ]; then
    echo "✗ ${sums} に version ${ver} の tarball のハッシュが揃っていません" >&2
    exit 1
  fi
done

cat > "$out" <<EOF
# Homebrew formula - safersync（Go 版）
#
#   ⚠ このファイルは packaging/homebrew/update-formula.sh が
#     dist/release/SHA256SUMS から自動生成する。手で編集しないこと。
#     tarball は gzip のタイムスタンプのためバイト再現性がなく、sha256 は
#     「実際に公開したリリースの値」でなければ検証に通らない。
#     リリース時は \`make release\` の後に \`make formula\` を実行して作り直し、
#     tap リポジトリ（kazpc1215/homebrew-tap）の Formula/safersync.rb へコピーする。
#
#   使い方（tap 公開後）:
#     brew install kazpc1215/tap/safersync
#
class Safersync < Formula
  # version は URL から走査させる（明示すると「URL と重複」で audit に怒られる）
  desc "Rsync の安全なラッパー（コピー → 照合）。進捗・残り時間表示付き"
  homepage "https://github.com/kazpc1215/safersync-go"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v${ver}/safersync-${ver}-darwin-arm64.tar.gz"
      sha256 "${darwin_arm}"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v${ver}/safersync-${ver}-darwin-amd64.tar.gz"
      sha256 "${darwin_amd}"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v${ver}/safersync-${ver}-linux-arm64.tar.gz"
      sha256 "${linux_arm}"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v${ver}/safersync-${ver}-linux-amd64.tar.gz"
      sha256 "${linux_amd}"
    end
  end

  def install
    bin.install "safersync"
  end

  def caveats
    <<~EOS
      rsync は safersync が内部で呼び出します。macOS / Linux に標準搭載されています。
      TUI ウィザードは \`safersync\`（引数なし）を対話端末で実行すると起動します。
    EOS
  end

  test do
    assert_match "safersync v${ver}", shell_output("#{bin}/safersync --version")
  end
end
EOF

echo "✓ formula を生成しました: $out (version ${ver})"
