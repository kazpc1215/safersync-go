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
  version "1.2.2"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "b2d0c8edb9b47e2d8b14235046425403b0daf1048357b09e2c6404dbd7a43e38"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "21a4f8d76d480c6ba0baa9db13dc16f6e69b5cf0ba684720ca42dcda488e78d1"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "6c5ba2ed377026265aed7be7a06c34eeccae7506cbcff620587318e4b8431411"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "acdfa2f114ce71b3a7ef7f0273ca5f9ba6d4fe566d36e2d99231d0652975f61e"
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
