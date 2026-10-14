#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
version=${1:-0.6.6}
output_dir=${2:-"$project_dir/release"}
binary=${CURSOR_PLUGIN_BINARY:-"$project_dir/cursor.so"}

arch=${GOARCH:-amd64}
case "$arch" in
	amd64|arm64) ;;
	*) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
[ -f "$binary" ] || { echo "set CURSOR_PLUGIN_BINARY to a Linux $arch cursor.so" >&2; exit 1; }
LC_ALL=C file "$binary" | case "$arch" in
	amd64) grep -Eq 'ELF 64-bit.*x86-64.*shared object|ELF 64-bit.*shared object.*x86-64' ;;
	arm64) grep -Eq 'ELF 64-bit.*ARM aarch64.*shared object|ELF 64-bit.*shared object.*ARM aarch64' ;;
esac || { echo "binary is not a Linux $arch shared library" >&2; exit 1; }
case "$output_dir" in
	/*) ;;
	*) output_dir="$PWD/$output_dir" ;;
esac

asset_name="cursor_${version}_linux_${arch}.zip"
manual_asset="cursor-plugin-${version}-linux-${arch}.tar.gz"
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT HUP INT TERM
mkdir -p "$output_dir"

install -m 0755 "$binary" "$staging/cursor.so"
(cd "$staging" && zip -q "$output_dir/$asset_name" cursor.so)

if command -v sha256sum >/dev/null 2>&1; then
	if [ -f "$output_dir/$manual_asset" ]; then
		(cd "$output_dir" && sha256sum "$asset_name" "$manual_asset" > checksums.txt)
	else
		(cd "$output_dir" && sha256sum "$asset_name" > checksums.txt)
	fi
else
	if [ -f "$output_dir/$manual_asset" ]; then
		(cd "$output_dir" && shasum -a 256 "$asset_name" "$manual_asset" > checksums.txt)
	else
		(cd "$output_dir" && shasum -a 256 "$asset_name" > checksums.txt)
	fi
fi

echo "$output_dir/$asset_name"
echo "$output_dir/checksums.txt"
