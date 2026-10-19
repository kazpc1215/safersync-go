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
  version "1.5.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-arm64.tar.gz"
      sha256 "df10e205e746aff1ea7992cff49c3f6c09b2afbd5b5086ab8a8d7c2b70ea3fb1"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-darwin-amd64.tar.gz"
      sha256 "3233ef7556a30ed49a0287d027970a071743f8afd3d8b949c12ae5bb27e07802"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-arm64.tar.gz"
      sha256 "622ef44b23aee7a3a0a75c7ec8d82b676752280662d877704e8523b30e43b5aa"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v#{version}/safersync-#{version}-linux-amd64.tar.gz"
      sha256 "0125f1706d0889e442948c073b053b5f95159083bc696f4dcab26221ba657c06"
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
