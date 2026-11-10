#!/usr/bin/env bash
# 构建 zchat。先做格式、测试和 vet 检查，除非设置 SKIP_TESTS=1。
#
#   ./scripts/build.sh                       # linux/amd64
#   ./scripts/build.sh windows/amd64
#   ./scripts/build.sh darwin/universal
#   ./scripts/build.sh linux/amd64,linux/arm64,windows/amd64
#   SKIP_TESTS=1 ./scripts/build.sh linux/amd64
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

platform="${1:-linux/amd64}"

if [[ -z "${SKIP_TESTS:-}" ]]; then
  echo "==> gofmt"
  unformatted="$(gofmt -l .)"
  if [[ -n "$unformatted" ]]; then
    echo "以下文件未格式化，请运行 gofmt -w："
    echo "$unformatted"
    exit 1
  fi

  echo "==> go test ./internal/..."
  go test ./internal/...

  echo "==> go vet ./..."
  go vet ./...

  echo "==> go tool mygo vet"
  go tool mygo vet
fi

echo "==> go tool mygo build -platform $platform"
go tool mygo build -platform "$platform"

out="$(sed -n 's/^[[:space:]]*"out":[[:space:]]*"\([^"]*\)".*/\1/p' mygo.json | head -n1)"
echo "==> 构建完成，产物在 ${out:-dist}/"
