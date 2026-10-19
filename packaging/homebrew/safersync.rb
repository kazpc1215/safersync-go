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
  desc "rsync の安全なラッパー（コピー → 照合）。進捗・残り時間表示付き"
  homepage "https://github.com/kazpc1215/safersync-go"
  version "1.5.1"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "0ce363dba2f46a08713404e5cd4c2de22ae97e9f687a84e9175aa891da506de7"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "56eb0a1fcef84f043cd18fb4012b716eab8db1a5db8d94838ea090701e655fd1"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "95a767e6fead6db900ded1d571ae2d3dae339e5d44463b94d779c046af09a70a"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "25611a877d5f654b5304238a87e0ab4fd464e35c71084c12c87b5f27a421da11"
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
