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

# ============= ТЕСТЫ ДЛЯ --name =============

echo "Testing named allocations..."

# 1. Базовое выделение именованных портов
echo "Test 1: Basic named allocations"
WEB_PORT=$($binary --name web)
API_PORT=$($binary --name api)
DB_PORT=$($binary --name db)

# Проверяем что порты разные
if [ "$WEB_PORT" = "$API_PORT" ] || [ "$WEB_PORT" = "$DB_PORT" ] || [ "$API_PORT" = "$DB_PORT" ]; then
  echo "ERROR: Named allocations returned duplicate ports"
  exit 1
fi

# 2. Повторные запросы возвращают тот же порт
echo "Test 2: Named allocations persistence"
if [ "$WEB_PORT" != "$($binary --name web)" ]; then
  echo "ERROR: 'web' allocation not persistent"
  exit 1
fi

if [ "$API_PORT" != "$($binary --name api)" ]; then
  echo "ERROR: 'api' allocation not persistent"
  exit 1
fi

# 3. --list показывает NAME колонку с разными именами
echo "Test 3: --list shows NAME column"
LIST_OUTPUT=$($binary --list)
if ! echo "$LIST_OUTPUT" | grep -qE "^$WEB_PORT[[:space:]]+web[[:space:]]"; then
  echo "ERROR: --list doesn't show 'web' allocation"
  exit 1
fi
if ! echo "$LIST_OUTPUT" | grep -qE "^$API_PORT[[:space:]]+api[[:space:]]"; then
  echo "ERROR: --list doesn't show 'api' allocation"
  exit 1
fi
if ! echo "$LIST_OUTPUT" | grep -qE "^$DB_PORT[[:space:]]+db[[:space:]]"; then
  echo "ERROR: --list doesn't show 'db' allocation"
  exit 1
fi

# 4. --lock --name блокирует конкретное имя
echo "Test 4: Lock named allocation"
if ! $binary --lock --name web | grep -qF "Locked port $WEB_PORT for 'web'"; then
  echo "ERROR: Failed to lock 'web' allocation"
  exit 1
fi

# 5. --unlock --name разблокирует конкретное имя
echo "Test 5: Unlock named allocation"
if ! $binary --unlock --name web | grep -qF "Unlocked port $WEB_PORT for 'web'"; then
  echo "ERROR: Failed to unlock 'web' allocation"
  exit 1
fi

# 6. --forget --name удаляет только конкретное имя
echo "Test 6: Forget specific named allocation"
$binary --forget --name api > /dev/null

LIST_AFTER_FORGET=$($binary --list)
if echo "$LIST_AFTER_FORGET" | grep -qE "^$API_PORT[[:space:]]+api[[:space:]]"; then
  echo "ERROR: 'api' allocation should have been deleted"
  exit 1
fi
if ! echo "$LIST_AFTER_FORGET" | grep -qE "^$WEB_PORT[[:space:]]+web[[:space:]]"; then
  echo "ERROR: 'web' allocation should still exist"
  exit 1
fi
if ! echo "$LIST_AFTER_FORGET" | grep -qE "^$DB_PORT[[:space:]]+db[[:space:]]"; then
  echo "ERROR: 'db' allocation should still exist"
  exit 1
fi

# 7. --forget без --name удаляет все имена в директории
echo "Test 7: Forget all allocations for directory"
$binary --forget > /dev/null

if ! $binary --list | grep -qF "No port allocations found."; then
  echo "ERROR: All allocations should have been deleted"
  exit 1
fi

# 8. Проверка default name 'main'
echo "Test 8: Default 'main' allocation"
DEFAULT_PORT=$($binary)
if [ "$DEFAULT_PORT" != "$($binary --name main)" ]; then
  echo "ERROR: Default allocation should work as 'main'"
  exit 1
fi

# 9. --name с пустым значением должна выдавать ошибку
echo "Test 9: Empty name validation"
if $binary --name="" 2>&1 | grep -qF "error:"; then
  echo "PASS: Empty name correctly rejected"
elif $binary --name "" 2>&1 | grep -qF "error:"; then
  echo "PASS: Empty name correctly rejected"
else
  echo "ERROR: Empty name should be rejected"
  exit 1
fi

echo "All named allocation tests passed!"
