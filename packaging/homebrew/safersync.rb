# Homebrew formula - safersync（Go 版）
#
#   ⚠ このファイルは packaging/homebrew/update-formula.sh が
#     dist/release/SHA256SUMS から自動生成する。手で編集しないこと。
#     tarball は gzip のタイムスタンプのためバイト再現性がなく、sha256 は
#     「実際に公開したリリースの値」でなければ検証に通らない。
#     リリース時は `make release` の後に `make formula` を実行して作り直し、
#     tap リポジトリ（kazpc1215/homebrew-tap）の Formula/safersync.rb へコピーする。
#
#   使い方（tap 公開後）:
#     brew install kazpc1215/tap/safersync
#
class Safersync < Formula
  # version は URL から走査させる（明示すると「URL と重複」で audit に怒られる）
  desc "Rsync の安全なラッパー（コピー → 照合）。進捗・残り時間表示付き"
  homepage "https://github.com/kazpc1215/safersync-go"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.6/safersync-1.5.6-darwin-arm64.tar.gz"
      sha256 "ae4520c40498f312da4ed6ac22f20d40b3f7b2757180f86ba0e0e147761b4789"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.6/safersync-1.5.6-darwin-amd64.tar.gz"
      sha256 "3b40ede01fa7c53fc0691a7dfda88584cd30175685859f26b449ba0917492da2"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.6/safersync-1.5.6-linux-arm64.tar.gz"
      sha256 "1551c9825dc54d7cb91b77550a78ac5ca4254cf2837121e851f2d91e5dd6fe11"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.6/safersync-1.5.6-linux-amd64.tar.gz"
      sha256 "e23b218470519999654d30c21d45be05754275c73a16c8704c65b8b29cf11bc5"
    end
  end

  def install
    bin.install "safersync"
  end

  def caveats
    <<~EOS
      rsync は safersync が内部で呼び出します。macOS / Linux に標準搭載されています。
      TUI ウィザードは `safersync`（引数なし）を対話端末で実行すると起動します。
    EOS
  end

  test do
    assert_match "safersync v1.5.6", shell_output("#{bin}/safersync --version")
  end
end
