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
  version "1.4.1"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "84e01b0559f86b2f64e3438cc4c9d8b009b5215bb5ca9d395e41c3af328a4267"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "db145bbc4d9c73e312730ccd24479ab78e5e311101ce2c66e5a1ca22d658dc96"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "1b00fa49c983df21d1c8c30d18c39cbaa83059247d5341ee8a094b7f04bb0022"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "84330976202d5b179d9af7e0d5ab69fb844bad70013008cf1764cf128d28bc37"
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
