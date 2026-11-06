# 发布流程

zchat 通过 GitHub Releases 分发，并使用 MyGo 的签名更新。发布由推送 `v*` tag 触发，CI 构建 macOS、Windows 和 Linux 产物并上传到草稿 Release，确认无误后手动发布。

本文以当前版本 `0.1.0` 为例；实际发布时替换成新版本号。

## 一次性配置

### 更新签名密钥

如果仓库还没有更新密钥：

```bash
go tool mygo keygen -o .update-keys
```

把 `.update-keys/mygo-update.pub` 的内容写入 `mygo.json` 的 `updates.publicKey`，把私钥内容保存为 GitHub 仓库 secret：

```bash
gh secret set MYGO_UPDATER_PRIVATE_KEY < .update-keys/mygo-update.key
```

私钥只保存在密码管理器和 GitHub Actions secret 中。`.update-keys/` 已被 `.gitignore` 忽略。

### 仓库信息

确认 `mygo.json` 中的仓库和版本正确：

```json
{
  "version": "0.1.0",
  "updates": {
    "publicKey": "...",
    "github": "zwdart/zchat"
  }
}
```

确认 `.github/workflows/release.yml` 存在，并且仓库 Actions 有写权限（workflow 已声明 `permissions: contents: write`）。

### 可选：macOS 签名与公证

默认的 macOS 构建使用临时签名（`-sign -`），只能本机运行，用户从网络下载后可能需要在「隐私与安全性」里手动放行。若要正式分发：

1. 在 `mygo.json` 的 `macos` 中加入 `signingIdentity`，例如：

   ```json
   "macos": {
     "minimumSystemVersion": "12.0",
     "signingIdentity": "Developer ID Application: Your Name (TEAMID)",
     "notarize": { "keychainProfile": "notary" }
   }
   ```

2. 配置证书和公证 secret（导入方式与 MyGo 官方示例相同，可参考 mygo-cloudmusic 的 `release.yml`）：

   - `MACOS_CERTIFICATE`：`.p12` 证书的 base64。
   - `MACOS_CERTIFICATE_PASSWORD`：证书密码。
   - `APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD`：公证账号。

3. 从 workflow 中删除 macOS 的 `-sign -`，让 `mygo build` 使用 `mygo.json` 中的签名配置。

没有这些 secret 时保持现状即可。

## 每次发布

### 1. 同步并验证 main

发布必须基于最新的 `main`，否则会漏掉别人刚推的提交：

```bash
git switch main
git pull --rebase origin main
go test ./internal/...
go test -race ./internal/...
go vet ./...
go tool mygo vet
```

工作区必须干净，所有改动已经提交。

### 2. 升版本并更新更新日志

修改 `mygo.json` 的 `version`，并在 `CHANGELOG.md` 顶部加入同名小节。小节标题必须与 tag 去掉 `v` 后完全一致，例如版本 `0.1.1` 对应：

```markdown
# 更新日志

## 0.1.1

- 修复……
- 新增……
```

内容用中文写给用户看，说明自上个版本以来的变化。`mygo build` 会把这一节写进更新清单，应用内的更新窗口会显示它；Release 页面也使用同一段文字作为说明。缺少对应小节时构建会失败。

提交版本号、更新日志以及本版本的所有代码：

```bash
git add -A
git commit -m "0.1.1"
```

可以先用 `git log v0.1.0..HEAD --oneline` 回顾这版包含的提交。

### 3. 先推送 main，再打 tag

必须分两步。若同时推送 main 和 tag，main 被拒绝时 tag 仍会推上去，CI 就会为一个不在 main 上的提交发布：

```bash
git push origin main

git tag v0.1.1
git push origin v0.1.1
```

如果 main 被拒绝，重新 `git pull --rebase origin main`，把新提交的内容补进更新日志，`git commit --amend` 后重新测试，再推 main，最后才打 tag。

如果 tag 已经推到了错误的提交：

```bash
# 先取消对应的 workflow run
git push origin :refs/tags/v0.1.1
gh release delete v0.1.1 --yes
# 修正后重新 tag 和 push
```

### 4. 等待 CI 构建

推送 tag 后，`.github/workflows/release.yml` 会：

1. 检查 tag 与 `mygo.json` 的版本一致。
2. 从 `CHANGELOG.md` 提取本版本说明，创建草稿 Release。
3. 在 macOS、Windows 和 Linux 上构建对应架构：
   - macOS：`darwin/arm64,darwin/amd64`（临时签名）
   - Windows：`windows/amd64,windows/arm64`，含 NSIS 安装器
   - Linux：`linux/amd64,linux/arm64`，含 tar.gz、Debian 包和 `PKGBUILD`
4. 用 `MYGO_UPDATER_PRIVATE_KEY` 签名更新产物并上传到草稿 Release。

查看运行状态：

```bash
gh run watch "$(gh run list --workflow release.yml -L1 --json databaseId -q '.[0].databaseId')" --exit-status
```

任何一个平台失败都不要发布；先修复并重新走流程。修复后如果要复用同一个版本号，需要删除旧 tag、旧草稿和失败的 run，再重新打 tag。

### 5. 发布草稿

确认草稿里的安装包、更新包和 `update-<platform>.json` 齐全，且 Release 说明正确：

```bash
gh release view v0.1.1
```

然后发布并标记为最新：

```bash
gh release edit v0.1.1 --draft=false --latest
```

已安装的旧版本会在下次检查更新时看到新版本。

### 6. 验证更新源

```bash
curl -sL https://github.com/zwdart/zchat/releases/latest/download/update-linux-amd64.json
curl -sL https://github.com/zwdart/zchat/releases/latest/download/update-darwin-arm64.json
curl -sL https://github.com/zwdart/zchat/releases/latest/download/update-windows-amd64.json
```

清单中的版本应为 `0.1.1`。再用一个旧版本安装包实际检查「设置 → 关于 → 检查更新」。

## 使用脚本发布

`scripts/release.sh` 封装了推送前的检查，但不会替你决定版本内容：

```bash
# 先完成版本号和 CHANGELOG 的修改并提交
./scripts/release.sh            # 检查后推送 main 和 tag
./scripts/release.sh --dry-run  # 只检查，不推送
```

脚本会：

- 确认在 `main` 分支且工作区干净。
- 确认 `mygo.json` 的版本在 `CHANGELOG.md` 中有对应小节。
- 运行格式检查、测试、vet 和 `mygo vet`。
- 确认 tag 尚不存在，且本地与 `origin/main` 一致。
- 先推送 `main`，成功后再创建并推送 `v<version>`。

CI 通过后仍需手动执行第 5、6 步发布和验证。

## 回滚

更新一旦发布，已经升级的用户不会自动降级。若新版本有严重问题：

1. 立即修复问题，发布更高版本（例如 `0.1.2`），让用户升级过去。
2. 必要时把有问题的 Release 改为 draft 或删除，阻止尚未更新的用户获取它：

   ```bash
   gh release edit v0.1.1 --draft=true
   ```

3. 不要删除已经发布的 tag 后复用同一版本号，更新签名和用户缓存都会产生冲突。

## 发布检查表

- [ ] `main` 已同步，工作区干净。
- [ ] `mygo.json` 版本已升级。
- [ ] `CHANGELOG.md` 有完全同名的小节，内容面向用户。
- [ ] 格式、测试、`go vet`、`go tool mygo vet` 全部通过。
- [ ] 版本提交已推送。
- [ ] tag 从最新的 `main` 创建，且 `v` 前缀与版本一致。
- [ ] 三个平台的 CI 全部通过。
- [ ] 草稿 Release 包含全部安装包、更新包和更新清单。
- [ ] 已发布并标记 latest。
- [ ] 更新清单能返回新版本。
- [ ] 用旧版本实际验证过一次更新。
