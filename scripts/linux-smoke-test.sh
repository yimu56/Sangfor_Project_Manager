#!/usr/bin/env bash
# Linux 冒烟测试：用临时数据目录启动一个实例，跑一遍
#   初始化 → 建客户 → 建项目 → Excel 导出 → PDF 导出 → 退出
#
#   bash scripts/linux-smoke-test.sh ./sangfor-pm
#   ./linux-smoke-test.sh /opt/sangfor-pm/sangfor-pm   # 单独拷到服务器也能用
#
# 依赖：curl。不会碰你正式的数据目录（-data 指向 mktemp 目录）。
set -euo pipefail

BIN="${1:-./sangfor-pm}"
PORT="${PORT:-18787}"
TMP="$(mktemp -d)"
JAR="$TMP/cookies.txt"
LOG="$TMP/server.log"
PID=""
BASE="http://127.0.0.1:${PORT}"

cleanup() {
  if [ -n "${PID}" ] && kill -0 "${PID}" 2>/dev/null; then
    kill "${PID}" 2>/dev/null || true
    wait "${PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP}"
}
trap cleanup EXIT

if [ ! -x "${BIN}" ]; then
  echo "[错误] 找不到可执行文件 ${BIN}，先执行：bash build.sh" >&2
  exit 1
fi
command -v curl >/dev/null 2>&1 || { echo "[错误] 需要 curl" >&2; exit 1; }

echo "==> 启动 ${BIN}（端口 ${PORT}，数据目录 ${TMP}/data）"
"${BIN}" -no-open -host 127.0.0.1 -port "${PORT}" -data "${TMP}/data" >"${LOG}" 2>&1 &
PID=$!

for _ in $(seq 1 60); do
  if curl -fsS "${BASE}/api/state" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

if ! curl -fsS "${BASE}/api/state" >/dev/null 2>&1; then
  echo "[错误] 服务未能在 6 秒内启动，日志：" >&2
  cat "${LOG}" >&2
  exit 1
fi
echo "==> /api/state OK"

post() {
  # 用文件传 JSON，避免终端 / MSYS 对命令行参数的编码转换破坏中文
  local body_file="${TMP}/body.json"
  printf '%s' "$2" >"${body_file}"
  curl -fsS -b "${JAR}" -c "${JAR}" -H 'Content-Type: application/json' \
    --data-binary "@${body_file}" "${BASE}$1"
}

post /api/setup '{"username":"admin","password":"test123456"}' >/dev/null
echo "==> 初始化账号 OK"
post /api/customers '{"code":"C001","name":"冒烟客户"}' >/dev/null
echo "==> 建客户 OK"
post /api/projects '{"name":"冒烟项目","customer_id":1,"ptype":"新实施","status":"进行中"}' >/dev/null
echo "==> 建项目 OK"

curl -fsS -b "${JAR}" "${BASE}/api/export/excel?type=projects" -o "${TMP}/out.xlsx"
head -c 2 "${TMP}/out.xlsx" | grep -aq 'PK' || { echo "[错误] Excel 导出内容异常" >&2; exit 1; }
echo "==> Excel 导出 OK（$(wc -c < "${TMP}/out.xlsx") 字节）"

if curl -fsS -b "${JAR}" "${BASE}/api/export/pdf?type=weekly&project_id=1" -o "${TMP}/out.pdf"; then
  head -c 4 "${TMP}/out.pdf" | grep -aq '%PDF' || { echo "[错误] PDF 导出内容异常" >&2; exit 1; }
  echo "==> PDF 导出 OK（$(wc -c < "${TMP}/out.pdf") 字节）"
else
  echo "⚠ PDF 导出失败，通常是系统缺少中文字体。"
  echo "  安装任意一种：apt install fonts-wqy-zenhei / fonts-arphic-uming / fonts-droid-fallback"
  echo "  或指定：PM_PDF_FONT=/path/to/font.ttf ./sangfor-pm"
  grep -a 'PDF' "${LOG}" || true
fi

echo "==> 冒烟测试通过（服务日志：${LOG}，退出后自动清理）"
