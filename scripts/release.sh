#!/usr/bin/env bash
# 发布 zchat：检查版本与更新日志，验证代码，先推送 main，再推送 tag。
# 推送 tag 后 GitHub Actions 会构建并上传草稿 Release，最后仍需手动发布。
#
#   ./scripts/release.sh            # 检查后推送 main 和 v<version>
#   ./scripts/release.sh --dry-run  # 只检查，不推送
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

dry_run=""
if [[ "${1:-}" == "--dry-run" ]]; then
  dry_run=1
elif [[ -n "${1:-}" ]]; then
  echo "用法：$0 [--dry-run]" >&2
  exit 2
fi

version="$(sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' mygo.json | head -n1)"
if [[ -z "$version" ]]; then
  echo "无法从 mygo.json 读取 version。" >&2
  exit 1
fi
tag="v$version"

echo "==> 发布版本 $version（tag $tag）"

branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$branch" != "main" ]]; then
  echo "当前分支是 $branch，请先在 main 上发布。" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "工作区不干净，请先提交或清理改动：" >&2
  git status --short >&2
  exit 1
fi

if ! grep -qx "## $version" CHANGELOG.md; then
  echo "CHANGELOG.md 缺少与版本号一致的 '## $version' 小节。" >&2
  exit 1
fi

if [[ -z "${SKIP_TESTS:-}" ]]; then
  echo "==> gofmt"
  unformatted="$(gofmt -l .)"
  if [[ -n "$unformatted" ]]; then
    echo "以下文件未格式化，请运行 gofmt -w：" >&2
    echo "$unformatted" >&2
    exit 1
  fi

  echo "==> go test ./internal/..."
  go test ./internal/...

  echo "==> go test -race ./internal/..."
  go test -race ./internal/...

  echo "==> go vet ./..."
  go vet ./...

  echo "==> go tool mygo vet"
  go tool mygo vet
fi

echo "==> 同步 origin/main"
git fetch origin main

if ! git rev-parse -q --verify refs/tags/"$tag" >/dev/null; then
  :
else
  echo "本地已存在 tag $tag。" >&2
  exit 1
fi

if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
  echo "远端已存在 tag $tag。若上次发布失败，请先删除旧 tag 和草稿。" >&2
  exit 1
fi

if [[ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]]; then
  echo "本地 main 与 origin/main 不一致，请先 git pull --rebase origin main。" >&2
  exit 1
fi

if [[ -n "$dry_run" ]]; then
  echo "==> 检查通过（--dry-run，未推送）"
  exit 0
fi

echo "==> 推送 main"
git push origin main

# 只有 main 推送成功后才创建 tag，避免 tag 指向未进入 main 的提交。
echo "==> 创建并推送 tag $tag"
git tag "$tag"
if ! git push origin "$tag"; then
  echo "tag 推送失败。本地 tag 仍存在，可用以下命令删除后重试：" >&2
  echo "  git tag -d $tag" >&2
  exit 1
fi

echo
echo "==> 已推送 $tag，等待 GitHub Actions 构建："
echo "  gh run watch \"\$(gh run list --workflow release.yml -L1 --json databaseId -q '.[0].databaseId')\" --exit-status"
echo
echo "全部通过后发布草稿："
echo "  gh release view $tag"
echo "  gh release edit $tag --draft=false --latest"
echo "  curl -sL https://github.com/zwdart/zchat/releases/latest/download/update-linux-amd64.json"
