#!/bin/sh
# Exercises real packages against temporary directories, never production paths.
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
arch=${GOARCH:-amd64}
binary=${CURSOR_PLUGIN_BINARY:-"$project_dir/cursor.so"}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
export GOARCH="$arch" CURSOR_PLUGIN_BINARY="$binary"
sh "$script_dir/build-release.sh" test "$work/out"
sh "$script_dir/build-store-release.sh" test "$work/out"
(cd "$work/out" && sha256sum -c checksums.txt)
tar -xzf "$work/out/cursor-plugin-test-linux-$arch.tar.gz" -C "$work"
package="$work/cursor-plugin-test-linux-$arch"
(cd "$package" && sha256sum -c SHA256SUMS)
unzip -q "$work/out/cursor_test_linux_$arch.zip" -d "$work/zip"
cmp "$binary" "$work/zip/cursor.so"
# A wrong architecture must fail rather than create a mislabeled release.
case "$arch" in arm64) wrong=amd64 ;; amd64) wrong=arm64 ;; esac
if GOARCH="$wrong" sh "$script_dir/build-release.sh" test "$work/wrong"; then
	echo 'incorrect architecture accepted' >&2; exit 1
fi
if GOARCH="$wrong" sh "$script_dir/build-store-release.sh" test "$work/wrong"; then
	echo 'incorrect store architecture accepted' >&2; exit 1
fi
if GOARCH=invalid sh "$script_dir/build-release.sh" test "$work/invalid"; then
	echo 'invalid architecture accepted' >&2; exit 1
fi
mkdir -p "$work/plugins" "$work/auth"
printf 'untouched auth sentinel\n' > "$work/auth/sentinel"
printf 'untouched config sentinel\n' > "$work/config.yaml"
sh "$package/install.sh" --plugins-dir "$work/plugins"
cmp "$binary" "$work/plugins/linux/$arch/cursor.so"
# Upgrade must preserve the previous binary before replacing it.
sh "$package/install.sh" --plugins-dir "$work/plugins"
set -- "$work/plugins/.cursor-backups/"*.so
[ "$#" -eq 1 ] && cmp "$binary" "$1"
sh "$package/uninstall.sh" --plugins-dir "$work/plugins"
[ ! -f "$work/plugins/linux/$arch/cursor.so" ]
set -- "$work/plugins/.cursor-uninstalled/"*.so
[ "$#" -eq 1 ] && cmp "$binary" "$1"
[ "$(cat "$work/auth/sentinel")" = 'untouched auth sentinel' ]
[ "$(cat "$work/config.yaml")" = 'untouched config sentinel' ]
echo "Packaging, checksum, architecture rejection, install, upgrade and uninstall tests passed ($arch)."
