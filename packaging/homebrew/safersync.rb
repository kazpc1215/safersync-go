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
  version "1.2.1"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "f7b62560ca309c1692ee065ab08d89c049cf3bc0071a912a7c576321efe51632"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "99d40728172ee896d423f5159a36423c1628fca7b9c24ebdb9ba3f06757f1aee"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "addad700914df92e973dcd4194c187b9d68e560e29e44ebd7bbd88c38836be0a"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "5a993e3eee0cdbba538a1b58003df25750908bc7d3a9bead73326ae6441a104c"
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
