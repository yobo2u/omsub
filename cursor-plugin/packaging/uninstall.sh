#!/bin/sh
set -eu

plugins_dir=${CLIPROXY_PLUGINS_DIR:-plugins}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--plugins-dir)
			[ "$#" -ge 2 ] || { echo "--plugins-dir requires a path" >&2; exit 2; }
			plugins_dir=$2
			shift 2
			;;
		--help|-h)
			echo "usage: $0 [--plugins-dir PATH]"
			exit 0
			;;
		*)
			echo "unknown argument: $1" >&2
			exit 2
			;;
	esac
done

case "$(uname -s)" in
	Linux) goos=linux ;;
	*) echo "unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64|amd64) goarch=amd64 ;;
	aarch64|arm64) goarch=arm64 ;;
	*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

target_file="$plugins_dir/$goos/$goarch/cursor.so"
[ -f "$target_file" ] || { echo "cursor plugin is not installed at $target_file"; exit 0; }

removed_dir="$plugins_dir/.cursor-uninstalled"
mkdir -p "$removed_dir"
removed_file="$removed_dir/cursor-$(date -u +%Y%m%dT%H%M%SZ).so"
mv "$target_file" "$removed_file"

echo "uninstalled cursor plugin; recoverable binary moved to $removed_file"
echo "disable plugins.configs.cursor.enabled, then restart CLIProxyAPI"
