# Homebrew formula - safersync（Go 版）
#
#   Homebrew tap（kazpc1215/homebrew-tap）に置くことを想定した formula。
#   tap リポジトリの Formula/safersync.rb にこの内容をコピーして使う。
#
#   使い方（tap 公開後）:
#     brew install kazpc1215/tap/safersync
#
#   リリースはチェックサム付き tarball。各プラットフォームの sha256 は
#   dist/release/SHA256SUMS と一致させる。
class Safersync < Formula
  desc "rsync の安全なラッパー（コピー → 照合）。進捗・残り時間表示付き"
  homepage "https://github.com/kazpc1215/safersync-go"
  version "1.3.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "fd6fdd48b84c73b006f218ee29c8ee43e6134f70249b99226d712fb1d01aaead"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "c6b96511a31aaf3d5def6f6fabb372a8a13e101d0cee5b0b8cb80bb505e7ed69"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "3edc00e4435a92313f636aa5f684d3bcbb203d61f498e8aedeb3a85ed0bc4810"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "a5334e17404547751aecdbae427184dcc1fa8f5861c12c1aef9c2bc3fa42c61a"
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
    assert_match "safersync v#{version}", shell_output("#{bin}/safersync --version")
  end
end
