#!/usr/bin/env bash
# 项目管理 · Linux / macOS 构建脚本
set -e
cd "$(dirname "$0")"
export CGO_ENABLED=0
GOOS=${GOOS:-linux}
GOARCH=${GOARCH:-amd64}
OUT=${OUT:-sangfor-pm}
echo "编译 ${GOOS}/${GOARCH} -> ${OUT}"
go build -trimpath -ldflags "-s -w" -o "${OUT}" .
chmod +x "${OUT}"
echo "完成：$(pwd)/${OUT}"
echo "运行：./${OUT}   （默认 http://127.0.0.1:8787，自动打开浏览器）"
echo "提示：Linux 服务器无桌面时使用 ./${OUT} -no-open -host 0.0.0.0"
