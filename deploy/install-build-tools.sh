#!/bin/sh
set -eu
case "$(uname -m)" in aarch64) task_arch=arm64;; x86_64) task_arch=x86_64;; *) echo 'Unsupported build architecture' >&2; exit 1;; esac
task_asset="railpack-v0.39.0-${task_arch}-unknown-linux-musl.tar.gz"
task_dir="$(mktemp -d)"
trap 'rm -rf "$task_dir"' EXIT
cd "$task_dir"
wget -q "https://github.com/railwayapp/railpack/releases/download/v0.39.0/$task_asset"
wget -q https://github.com/railwayapp/railpack/releases/download/v0.39.0/checksums.txt
awk -v asset="$task_asset" '$2 == asset || $2 == "*" asset {print}' checksums.txt > selected.sha256
test -s selected.sha256
sha256sum -c selected.sha256
tar xzf "$task_asset"
install -m 0755 railpack /usr/local/bin/railpack
