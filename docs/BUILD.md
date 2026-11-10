# 构建指南

本文说明如何在本地开发、测试和打包 zchat。发布到 GitHub Releases 的完整流程见 [RELEASE.md](RELEASE.md)。

## 环境要求

- Go 1.27.3 或更新版本（以 `go.mod` 为准）。
- Linux 需要 gcc 等基础构建工具；MyGo 使用 `purego`，不需要 cgo。
- 打包 Windows 安装器需要 [NSIS](https://nsis.sourceforge.io/)；在 Debian/Ubuntu 上安装：

  ```bash
  sudo apt-get update
  sudo apt-get install -y nsis
  ```

- 打包 macOS 的 `.app`、`.dmg` 和签名需要 macOS。
- 上传更新需要 GitHub CLI（`gh`）和更新签名私钥，见下文。

## 获取依赖

```bash
go mod download
go tool mygo doctor
```

`go tool mygo doctor` 会检查 Go 版本、平台工具链和打包环境。MyGo 版本固定在 `go.mod` 的 `tool` 指令中，不需要单独安装。

## 开发运行

```bash
go run .
```

程序使用原生窗口，不依赖浏览器或 WebView。首次运行会在系统用户数据目录创建设置、状态和消息文件。

## 提交前检查

```bash
gofmt -l .
go test ./internal/...
go test -race ./internal/...
go vet ./...
go tool mygo vet
```

- `gofmt -l .` 应当没有输出。
- `go tool mygo vet` 会额外检查 MyGo 原生 UI 的跨线程生命周期问题。
- 新增或修改 UI 测试后，用 `-count=5` 重复运行，确认不会偶发失败：

  ```bash
  go test ./internal/app -run TestName -count=5
  ```

## 本地打包

```bash
# 当前平台，默认 linux/amd64
go tool mygo build

# 指定一个或多个平台
go tool mygo build -platform linux/amd64
go tool mygo build -platform windows/amd64
go tool mygo build -platform darwin/universal
go tool mygo build -platform linux/amd64,linux/arm64,windows/amd64,darwin/arm64,darwin/amd64

# 输出到其他目录
go tool mygo build -o dist -platform linux/amd64
```

也可以使用脚本，它会先执行格式检查、测试和 vet，再调用 `mygo build`：

```bash
./scripts/build.sh                       # linux/amd64
./scripts/build.sh windows/amd64
SKIP_TESTS=1 ./scripts/build.sh darwin/universal
```

产物默认写入 `build/`（由 `mygo.json` 的 `out` 决定）：

| 平台 | 产物 |
| --- | --- |
| Linux | 可执行文件、`zchat-<version>-linux-<arch>.tar.gz`、Debian 包、`PKGBUILD` |
| Windows | `.exe`、`zchat-setup-<version>-<arch>.exe` 安装器 |
| macOS | `zchat.app`、`zchat-<version>-<arch>.dmg` |

`zchat.app` 只有在设置了 `macos.signingIdentity` 或传入 `-sign` 时才会用 Developer ID 签名。没有证书时可以用临时签名本机测试：

```bash
go tool mygo build -platform darwin/arm64 -sign - -skip-notarize
```

Linux 的 tar.gz 内含 `install.sh`，安装到 `~/.local`，因此应用可以自行更新。Debian 包和 Arch 的 `PKGBUILD` 也在同一次构建中生成。

## 更新签名密钥

MyGo 用 Ed25519 密钥对签名更新清单。公钥写在 `mygo.json` 的 `updates.publicKey`，私钥只放在发布环境里。

首次创建密钥对：

```bash
go tool mygo keygen -o .update-keys
```

- `.update-keys/mygo-update.pub` 的内容就是公钥，填到 `mygo.json`。
- `.update-keys/mygo-update.key` 是私钥。把它保存到密码管理器，并配置为 GitHub 仓库的 Actions secret `MYGO_UPDATER_PRIVATE_KEY`。
- `.update-keys/` 已加入 `.gitignore`，**不要提交或上传私钥**。私钥泄露后，任何人都能伪造你的更新。

本地带更新产物构建并上传：

```bash
export MYGO_UPDATER_PRIVATE_KEY="$(cat .update-keys/mygo-update.key)"
export GH_TOKEN="$(gh auth token)"
go tool mygo build -platform linux/amd64 -upload
```

`-upload` 会把安装包、增量更新和 `update-<platform>.json` 上传到 `mygo.json` 中 `updates.github` 指向仓库的草稿 Release。正式发布由 tag 触发的 GitHub Actions 完成。

旧版本的更新清单是生成增量更新所必需的；`mygo build` 会从已发布的 Release 读取它们。首次发布没有旧版本，只生成完整更新包。

## 常见问题

**端口被占用**

发现、聊天和文件共享默认分别使用 `42223/udp`、`42224/tcp`、`42230/tcp`。可以在「设置 → 网络」里修改，或检查是否有另一个 zchat 正在运行。

**其他设备发现不到本机**

- 确认在同一子网，且路由器没有开启 AP 隔离或客户端隔离。
- 允许系统防火墙放行 zchat 的入站 UDP/TCP 连接。
- 组播不可用时，用「通过 IP 连接」输入对方的 `IP:42224`。

**Linux 下托盘图标不显示**

需要桌面环境提供 StatusNotifier/AppIndicator 支持。没有托盘时，关闭窗口的行为仍可在设置中调整。

**macOS 提示应用已损坏**

本机构建使用临时签名时，可能需要在「系统设置 → 隐私与安全性」中允许打开；正式发布应配置 Developer ID 并完成公证。

## 目录结构

```text
.
├── main.go                 程序入口
├── internal/
│   ├── app/                应用状态、网络回调与原生 UI
│   ├── lan/                UDP 发现与 TCP 连接
│   ├── model/              设置、会话、消息模型
│   ├── netutil/            地址与网卡工具
│   ├── proto/              帧协议与编解码
│   ├── share/              局域网 HTTP 共享与二维码
│   └── store/              设置、状态与消息持久化
├── resources/              应用图标
├── scripts/                构建与发布脚本
├── docs/                   设计、功能、构建与发布文档
└── mygo.json               应用元数据、版本与更新配置
```
