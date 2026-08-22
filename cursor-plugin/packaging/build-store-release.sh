#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
version=${1:-0.5.6}
output_dir=${2:-"$project_dir/release"}
binary=${CURSOR_PLUGIN_BINARY:-"$project_dir/cursor.so"}

[ -f "$binary" ] || { echo "set CURSOR_PLUGIN_BINARY to a Linux amd64 cursor.so" >&2; exit 1; }

asset_name="cursor_${version}_linux_amd64.zip"
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT HUP INT TERM
mkdir -p "$output_dir"

install -m 0755 "$binary" "$staging/cursor.so"
(cd "$staging" && zip -q "$output_dir/$asset_name" cursor.so)

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$output_dir" && sha256sum "$asset_name" > checksums.txt)
else
	(cd "$output_dir" && shasum -a 256 "$asset_name" > checksums.txt)
fi

echo "$output_dir/$asset_name"
echo "$output_dir/checksums.txt"
