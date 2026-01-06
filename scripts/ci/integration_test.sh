#!/usr/bin/env bash

set -euo pipefail

binary="${1:-}"
if [ -z "$binary" ]; then
  echo "usage: $0 /path/to/port-selector"
  exit 2
fi

if [ ! -x "$binary" ]; then
  echo "port-selector binary not found or not executable: $binary"
  exit 1
fi
binary="$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")"

tmp_dir="$(mktemp -d 2>/dev/null || mktemp -d -t port-selector)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

export HOME="$tmp_dir/home"
export XDG_CONFIG_HOME="$HOME/.config"
mkdir -p "$XDG_CONFIG_HOME"

work_dir="$tmp_dir/project"
mkdir -p "$work_dir"
cd "$work_dir"

expect_output() {
  local expected="$1"
  shift
  local out
  out="$($binary "$@")"
  if [ "$out" != "$expected" ]; then
    echo "expected '$expected', got '$out'"
    exit 1
  fi
}

expect_output "3000"
expect_output "3000"

if ! "$binary" --list | grep -qE '^3000[[:space:]]'; then
  echo "expected port 3000 in list"
  exit 1
fi

"$binary" --forget > /dev/null

if ! "$binary" --list | grep -qF "No port allocations found."; then
  echo "expected no allocations after --forget"
  exit 1
fi

if ! "$binary" --lock 3010 | grep -qF "Locked port 3010"; then
  echo "expected --lock 3010 to succeed"
  exit 1
fi

expect_output "3010"
