#!/usr/bin/env bash
# 项目管理 · Linux 构建脚本（支持交叉编译 amd64 / arm64）
#
# 用法：
#   bash build.sh                                  # 按当前系统编译
#   chmod +x build.sh && ./build.sh                # 已加执行位时也可直接运行
#   GOOS=linux GOARCH=arm64 bash build.sh          # 交叉编译
#   OUT=dist/sangfor-pm-linux-amd64 bash build.sh  # 指定输出路径
#
# 说明：依赖 modernc.org/sqlite（纯 Go），全程 CGO_ENABLED=0，无需 C 编译器。
set -euo pipefail
cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
  echo "[错误] 未找到 go，请先安装 Go 1.26+：https://go.dev/dl/" >&2
  exit 1
fi

export CGO_ENABLED=0
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
OUT="${OUT:-sangfor-pm}"

go_version="$(go env GOVERSION 2>/dev/null || go version | awk '{print $3}')"
echo "==> 使用 ${go_version}"
echo "==> 编译 ${GOOS}/${GOARCH} -> ${OUT}"

mkdir -p "$(dirname "${OUT}")"
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "${OUT}" .
chmod +x "${OUT}" 2>/dev/null || true

size="$(wc -c < "${OUT}" | tr -d ' ')"
echo "==> 完成：$(cd "$(dirname "${OUT}")" && pwd)/$(basename "${OUT}")（${size} 字节）"

if [ "${GOOS}" = "linux" ] || [ "${GOOS}" = "darwin" ]; then
  echo "    启动：./${OUT}"
  echo "    无桌面 / 服务器：./${OUT} -no-open -host 0.0.0.0"
  echo "    部署（systemd）：见 docs/linux-deploy.md"
fi
