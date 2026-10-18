#!/bin/sh
#
# install.sh - safersync（Go 版）の最新リリースを取得して ~/.local/bin に置く
#
#   使い方:
#     curl -fsSL https://raw.githubusercontent.com/kazpc1215/safersync-go/main/install.sh | sh
#   または:
#     git clone ...; ./install.sh [バージョン指定 例: v1.2.0]
#
#   依存は curl（macOS / Linux にはほぼ必ずある）と tar のみ。
#   Homebrew・管理者権限は不要。 ~/.local/bin が PATH に無い場合は最後に案内する。
#
set -eu

REPO="kazpc1215/safersync-go"
APP="safersync"
PREFIX="${PREFIX:-$HOME/.local/bin}"

echo "safersync インストーラ（${REPO}）"

# OS / アーキの判定（Go の GOOS/GOARCH 命名）
os=$(uname -s); arch=$(uname -m)
case "$os" in
  Darwin) os=darwin; case "$arch" in arm64|aarch64) g=arm64;; *) g=amd64;; esac ;;
  Linux)  os=linux; case "$arch" in arm64|aarch64) g=arm64;; *) g=amd64;; esac ;;
  *) echo "✗ 未対応の OS: $os（macOS または Linux で実行してください）" >&2; exit 1 ;;
esac

# バージョンの決定（引数 -> 環境変数 -> 最新リリース）
ver="${1:-${SAFERSYNC_VERSION:-}}"
if [ -z "$ver" ]; then
  echo "  最新リリースを確認中..."
  api="https://api.github.com/repos/$REPO/releases/latest"
  ver=$(curl -fsSL "$api" 2>/dev/null | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/') || ver=""
  if [ -z "$ver" ]; then
    echo "✗ 最新リリースを取得できませんでした。ネットワークを確認するか、バージョンを指定してください" >&2
    echo "  例: ./install.sh v1.2.0" >&2
    exit 1
  fi
fi
echo "  対象: $ver / ${os}-${g}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
url="https://github.com/$REPO/releases/download/$ver/${APP}-${ver#v}-${os}-${g}.tar.gz"
echo "  ダウンロード中: $url"
curl -fsSL "$url" -o "$tmp/${APP}.tar.gz" || {
  echo "✗ ダウンロードできませんでした（$url）" >&2
  exit 1
}
tar -C "$tmp" -xzf "$tmp/${APP}.tar.gz"

mkdir -p "$PREFIX"
mv "${tmp:?}/$APP" "$PREFIX/$APP"
chmod +x "$PREFIX/$APP"

echo "✓ インストール完了: $PREFIX/$APP"
if ! command -v "$APP" >/dev/null 2>&1; then
  case ":$PATH:" in
    *":$PREFIX:"*) ;;
    *) echo "  ※ PATH に $PREFIX がありません。以下をシェル設定に追加してください:" >&2
       echo "      export PATH=\"$PREFIX:\$PATH\"" >&2 ;;
  esac
fi
