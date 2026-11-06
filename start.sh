#!/usr/bin/env bash
# 一键启动：没有二进制就先编译；无图形界面时自动加 -no-open，避免在服务器上尝试拉起浏览器。
#
#   ./start.sh                 # 默认 0.0.0.0:8787
#   ./start.sh -host 127.0.0.1 -port 9000
#   PM_DATA_DIR=/var/lib/sangfor-pm ./start.sh -no-open
set -euo pipefail
cd "$(dirname "$0")"

BIN="${BIN:-./sangfor-pm}"
if [ ! -x "${BIN}" ]; then
  echo "未找到 ${BIN}，先执行构建…"
  bash ./build.sh
fi

args=()
if [ "$(uname -s 2>/dev/null || echo unknown)" = "Linux" ] &&
   [ -z "${DISPLAY:-}" ] && [ -z "${WAYLAND_DISPLAY:-}" ]; then
  args+=("-no-open")
fi

exec "${BIN}" "${args[@]}" "$@"
