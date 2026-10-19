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
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.5/safersync-1.5.5-darwin-arm64.tar.gz"
      sha256 "4be6f024a16765bee3b19a10feb15c0517797137cb7e5d3a756bf76f85d7f062"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.5/safersync-1.5.5-darwin-amd64.tar.gz"
      sha256 "000a865d7efc59f40f9cb09962d52a0f9d0136065e75165e4c2ed296fb35420c"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.5/safersync-1.5.5-linux-arm64.tar.gz"
      sha256 "e8c15e929d70c552161a1c4eacc2b2a3df4c10c288f84a2b6445b23de6b328c3"
    else
      url "https://github.com/kazpc1215/safersync-go/releases/download/v1.5.5/safersync-1.5.5-linux-amd64.tar.gz"
      sha256 "ef57045b1681f1d35c4c5db39319aee6761541f2fe399e1c77b8cd6de0d63164"
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
    assert_match "safersync v1.5.5", shell_output("#{bin}/safersync --version")
  end
end
