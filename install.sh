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
#   ダウンロードした tarball はリリースに添付された SHA256SUMS と照合し、
#   一致しなければ展開しない（破損・改ざんされたものを実行しない）。
#   Homebrew・管理者権限は不要。 ~/.local/bin が PATH に無い場合は最後に案内する。
#
#   環境変数:
#     PREFIX                 インストール先（既定: $HOME/.local/bin）
#     SAFERSYNC_VERSION      バージョン（引数と同じ。例: v1.2.0）
#     SAFERSYNC_RELEASES_URL ダウンロード元のミラー・テスト用
#                            （既定: https://github.com/$REPO/releases/download）
#
set -eu

REPO="kazpc1215/safersync-go"
APP="safersync"
PREFIX="${PREFIX:-$HOME/.local/bin}"
RELEASES_URL="${SAFERSYNC_RELEASES_URL:-https://github.com/$REPO/releases/download}"

echo "safersync インストーラ（${REPO}）"

# OS / アーキの判定（Go の GOOS/GOARCH 命名）
os=$(uname -s); arch=$(uname -m)
case "$os" in
  Darwin) os=darwin; case "$arch" in arm64|aarch64) g=arm64;; *) g=amd64;; esac ;;
  Linux)  os=linux; case "$arch" in arm64|aarch64) g=arm64;; *) g=amd64;; esac ;;
  *) echo "✗ 未対応の OS: ${os}（macOS または Linux で実行してください）" >&2; exit 1 ;;
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

file="${APP}-${ver#v}-${os}-${g}.tar.gz"
file_url="$RELEASES_URL/$ver/$file"
sums_url="$RELEASES_URL/$ver/SHA256SUMS"

echo "  ダウンロード中: $file_url"
curl -fsSL "$file_url" -o "$tmp/$file" || {
  echo "✗ ダウンロードできませんでした（${file_url}）" >&2
  exit 1
}

# チェックサムの検証（壊れた・差し替えられた tarball を展開しない）
echo "  チェックサムを確認中..."
curl -fsSL "$sums_url" -o "$tmp/SHA256SUMS" || {
  echo "✗ SHA256SUMS を取得できませんでした（${sums_url}）" >&2
  echo "  安全のためインストールを中止します" >&2
  exit 1
}
want=$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/SHA256SUMS")
if [ -z "$want" ]; then
  echo "✗ SHA256SUMS に $file のハッシュがありません" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$file" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$file" | awk '{print $1}')
else
  echo "✗ sha256sum / shasum が見つからないため検証できません" >&2
  exit 1
fi
if [ "$want" != "$got" ]; then
  echo "✗ チェックサムが一致しません（ダウンロードの破損、または改ざんの可能性）" >&2
  echo "  期待: $want" >&2
  echo "  実際: $got" >&2
  exit 1
fi
echo "  ✓ チェックサム OK（sha256: $(printf %.12s "$got")…）"

tar -C "$tmp" -xzf "$tmp/$file"

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
