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

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.4/safersync-1.5.4-darwin-arm64.tar.gz"
      sha256 "7c255fa0b903ea775e59edd052b24aa947c46f898f89bee01997757b9ae4ce8a"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.4/safersync-1.5.4-darwin-amd64.tar.gz"
      sha256 "91a8ccfc468816698ee21c802c67b0d07c5e2eb023a43dd04741769668c99b44"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.4/safersync-1.5.4-linux-arm64.tar.gz"
      sha256 "ffa2d1701b466d5d7e6443eaceb3a99671a0ea59494ad8b027a84434448e9446"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.4/safersync-1.5.4-linux-amd64.tar.gz"
      sha256 "89eff24df016f826bed82bc02aca1df9ae6a64a10f87614888637ccee4b8fa88"
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
    assert_match "safersync v1.5.4", shell_output("#{bin}/safersync --version")
  end
end
