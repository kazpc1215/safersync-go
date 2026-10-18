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
  version "1.2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "8ce1784168f561ff5c188ef9e3bc92872d3164bd2c48dcb9e7e95737a8df50a5"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "b66eba3bff389abb6e6c73bb0b433b12002df309944c8e834a18b1286b692e60"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "a202ca66a7d23db14154f7e5bf2937735221b3adb1d760cb8748a29b4a63ee26"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "c29951a49342a6e0c9f3c31cb9084cda2e74a3d861197d9a45ac638796f2910a"
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
