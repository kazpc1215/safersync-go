#!/bin/sh
#
# test-install.sh - install.sh のスモークテスト（ネットワーク不要）
#
#   現在のプラットフォーム向けにその場で tarball を作り、file:// の
#   リリースミラー経由で install.sh を実行する。導入とバージョン確認、
#   および不正なバージョンの拒否を検証する。
#
#   使い方: sh scripts/test-install.sh
#
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

ver=$(sed -n 's/^VERSION := //p' "$root/Makefile" | head -1)
if [ -z "$ver" ]; then
  echo "✗ Makefile から VERSION を取得できません" >&2
  exit 1
fi

os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  *) echo "✗ 未対応 OS: $os" >&2; exit 1 ;;
esac
case "$arch" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=amd64 ;;
  *) echo "✗ 未対応アーキ: $arch" >&2; exit 1 ;;
esac

echo "== install.sh スモークテスト（v$ver / $os-${arch}）=="

# テスト用バイナリと tarball を作る（版は Makefile に合わせる）
(
  cd "$root"
  CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/kazpc1215/safersync-go/internal/version.Version=$ver" \
    -o "$work/safersync" .
)
tar -C "$work" -czf "$work/safersync-$ver-$os-$arch.tar.gz" safersync

# リリースミラー（<base>/v<ver>/...）を組み立てる
mirror="$work/mirror"
rel="$mirror/v$ver"
mkdir -p "$rel"
mv "$work/safersync-$ver-$os-$arch.tar.gz" "$rel/"
(
  cd "$rel"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- *.tar.gz > SHA256SUMS
  else
    shasum -a 256 -- *.tar.gz > SHA256SUMS
  fi
)

prefix="$work/prefix"

echo "-- 正常系: 導入とバージョン確認"
SAFERSYNC_RELEASES_URL="file://$mirror" PREFIX="$prefix" sh "$root/install.sh" "v$ver"

got=$("$prefix/safersync" --version)
want="safersync v$ver"
if [ "$got" != "$want" ]; then
  echo "✗ --version = '$got'（期待 '$want'）" >&2
  exit 1
fi

echo "-- 異常系: 不正なバージョンを拒否するか"
if SAFERSYNC_RELEASES_URL="file://$mirror" PREFIX="$prefix" sh "$root/install.sh" "not-a-version" 2>/dev/null; then
  echo "✗ 不正なバージョンを拒否しなかった" >&2
  exit 1
fi

echo "✓ install.sh スモークテスト OK"
