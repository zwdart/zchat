# zchat 局域网协议接口文档

> 协议版本：`v1`（`internal/proto.Version = 1`）
>
> 适用范围：zchat 当前代码中的设备发现、聊天消息、图片/文件传输和 HTTP 回退。
>
> 本文面向需要跨语言实现 zchat 客户端、网关或兼容层的开发者。文中 **MUST / SHOULD / MAY** 的含义遵循 RFC 2119。
>
> 实现参考：
>
> - `internal/proto/proto.go`：帧信封、类型和版本校验
> - `internal/lan/lan.go`、`internal/lan/discovery.go`、`internal/lan/conn.go`：UDP 发现、TCP 连接、帧分发
> - `internal/model/model.go`：Peer、Message、File 等线上数据结构
> - `internal/transfer/transfer.go`：TCP 附件拉取与断点续传
> - `internal/share/share.go`：HTTP 附件回退、目录共享和上传
> - `internal/app/network.go`、`internal/app/files.go`、`internal/app/groups.go`：业务语义

---

## 1. 总览

zchat 不使用中心服务器，所有数据都在同一局域网内的设备之间直接传输。协议由四个平面组成：

```
┌─────────────────────────────────────────────────────────────┐
│ 1. 发现平面   UDP 组播/广播，JSON 小包，默认 42223           │
├─────────────────────────────────────────────────────────────┤
│ 2. 聊天平面   TCP 长连接，一行一个 JSON 帧，默认 42224       │
├─────────────────────────────────────────────────────────────┤
│ 3. 附件平面   TCP 拉取，JSON 请求头 + 裸二进制流，默认 42225 │
├─────────────────────────────────────────────────────────────┤
│ 4. 回退平面   HTTP 文件/附件服务，默认 42230                 │
└─────────────────────────────────────────────────────────────┘
```

关键设计：

- **消息与文件分离**：聊天帧只携带消息和附件元数据；文件正文永远不进入聊天帧。
- **发送方提供服务，接收方主动拉取**：接收方根据消息里的 `file.token` 连接发送方的附件端口，从已保存的偏移量继续下载。
- **HTTP 回退**：消息里可以同时带 `file.url`，TCP 失败时改用 HTTP `Range` 续传；旧版本只认识 URL。
- **无加密、无身份认证**：协议默认运行在可信局域网内，令牌是唯一的访问控制手段。

### 1.1 默认端口

| 用途 | 传输层 | 默认端口 | 是否可在设置中修改 | 说明 |
| --- | --- | ---: | --- | --- |
| 设备发现 | UDP | 42223 | 只能整体开关，不能改端口 | 组播 + 定向广播 + 受限广播 |
| 聊天/控制连接 | TCP | 42224 | 是 | 每对设备一条长连接 |
| TCP 附件传输 | TCP | 42225 | 是 | 端口被占用时自动改用随机空闲端口 |
| HTTP 附件/目录共享 | TCP | 42230 | 是 | 端口被占用时启动失败 |

> 端口都绑定在 `:port`（所有网卡）。发现和连接实际以 IPv4 为主。

### 1.2 标识符与令牌

| 名称 | 格式 | 生成方 | 说明 |
| --- | --- | --- | --- |
| 设备 ID `id` | `u_` + 32 位十六进制 | 每台设备首次运行 | 持久化，视为不透明字符串 |
| 消息 ID `id` | `m_` + 32 位十六进制 | 发送方 | 接收方可对空 ID 自行生成 |
| 群 ID `id` | `g_` + 32 位十六进制 | 群创建者 | **以 `g_` 前缀识别群会话** |
| 启动实例 `instance` | `boot_` + 32 位十六进制 | 每次启动 | 仅用于发现包区分重启，不参与身份 |
| TCP 附件令牌 `token` | `t_` + 36 位十六进制 | 发送方 | 附件端口查找文件用 |
| HTTP 附件令牌 | 32 位十六进制 | 发送方 | 位于 `/t/<token>/<name>`，与 TCP 令牌不同 |

实现方 **MUST NOT** 依赖 ID 或令牌的长度、前缀以外的内部结构；除 `g_` 用于判断群会话外，其余都按不透明字符串处理。

---

## 2. 通用约定

### 2.1 编码与 JSON

- 所有文本使用 **UTF-8**。
- 字段名使用 **lowerCamelCase**。
- 整数为 JSON number；64 位整数（文件大小、时间戳）在跨语言实现中应使用 64 位有符号整数。
- 时间有两种形式：
  - 帧内业务时间：Go `time.Time` 的 JSON 形式，即 **RFC 3339 / ISO 8601**，可带纳秒和时区，例如 `2026-11-14T10:20:30.123456789+08:00`。
  - 信封 `time`、回执 `time`、`ts`：**Unix 毫秒**。
- 接收方 **MUST** 忽略未知字段；新增字段 **SHOULD** 保持向后兼容。
- 接收方遇到未知 `type` **SHOULD** 忽略该帧并继续读下一帧，而不是断开连接。
- 接收方遇到未知 `kind` 可按 `text` 处理（当前实现即如此），但不要崩溃。

### 2.2 行式帧

聊天协议每帧是一行 JSON，以 `\n` 结束：

```
<JSON>\n
```

- 发送方 **MUST** 使用单个 `\n` 分帧。
- 接收方 **SHOULD** 同时接受 `\n` 和 `\r\n`（Go 的 `bufio.ReadLine` 会去掉行尾 `\r`）。
- JSON 字符串中的换行会被转义为 `\n`，不会破坏分帧。
- 单帧最大 **2 MiB（2,097,152 字节，含 JSON 但不含行尾）**。超限必须拒绝并关闭连接。
- 没有长度前缀、没有压缩、没有分块。

### 2.3 协议版本

- 信封字段 `v` 表示协议版本，当前唯一合法值为 `1`。
- 握手帧和后续每一帧都要带 `v`。
- 接收方 **MUST** 拒绝 `v != 1` 的帧，并关闭连接。当前实现没有降级协商。
- 发现包中的 `v` 也是协议版本；`version` 字段才是应用版本，二者不能混淆。

### 2.4 安全模型

- 没有 TLS、签名、HMAC 或设备认证。
- 任何局域网设备都可以伪造发现包、`from`、消息内容、ACK、撤回和群操作。
- 附件令牌是 bearer token：拿到令牌即可下载，不校验来源 IP。
- 协议假设局域网可信。需要跨不可信网络时，应使用 VPN 或额外的安全隧道。

---

## 3. 设备发现协议（UDP）

### 3.1 传输方式

发现包是小型 JSON 对象，通过三种方式发送，以提高在访客 Wi-Fi、屏蔽组播的企业网络中的成功率：

1. **IPv4 组播**：`239.255.77.77:42223`，在每个支持组播的非回环接口上加入组。
2. **定向广播**：按每个接口的 IPv4 地址和掩码计算广播地址，目标端口同上。
3. **受限广播**：最后向 `255.255.255.255:42223` 发送一次作为兜底。

监听端：

- 优先绑定固定 UDP 端口 `42223`。
- 如果该端口被占用，仍用随机端口创建发送 socket，但**无法接收**其他设备的发现包；此时只能手动连接。
- 接收缓冲区为 4096 字节，发送包应远小于此值。
- 发现包**不携带头像**，头像只在 TCP 握手的 `hello` 里交换。

### 3.2 发现包结构

```json
{
  "app": "zchat",
  "v": 1,
  "kind": "hello",
  "id": "u_5f0c...",
  "instance": "boot_9a31...",
  "name": "Alice",
  "signature": "在工位",
  "status": "online",
  "version": "1.4.0",
  "platform": "windows/amd64",
  "port": 42224,
  "transferPort": 42225,
  "ts": 1763096430123
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `app` | string | 是 | 固定为 `"zchat"`；其他值丢弃 |
| `v` | int | 是 | 协议版本，必须为 `1` |
| `kind` | string | 是 | `hello` / `probe` / `bye` |
| `id` | string | 是 | 设备 ID；为空或等于自己则丢弃 |
| `instance` | string | 否 | 本次启动实例 ID；用于诊断和区分重启 |
| `name` | string | 是 | 设备/用户名 |
| `signature` | string | 否 | 个性签名 |
| `status` | string | 是 | `online` / `away` / `busy` / `dnd` / `invisible` / `offline` |
| `version` | string | 否 | 应用版本，不是协议版本 |
| `platform` | string | 否 | 例如 `windows/amd64`、`darwin/arm64` |
| `port` | int | 是 | 聊天 TCP 端口，1–65535 |
| `transferPort` | int | 否 | TCP 附件端口；`0` 或缺失表示不支持 TCP 附件 |
| `ts` | int64 | 是 | 发送时间，Unix 毫秒 |

接收规则：

- `app != "zchat"`、`v != 1`、`id == ""`、`id` 等于自身：丢弃。
- `port <= 0 || port > 65535`：丢弃。
- `status` 空：按 `online` 处理。
- `status == "invisible"`：在发现表中按 `offline` 处理，但已有 TCP 连接不受影响。
- 源 IP 以 UDP 包的源地址为准；发现包本身不携带 `ip` 字段。
- `kind == "bye"`：直接把该设备标记离线。
- `kind == "probe"`：收到方应立即广播一个 `hello`，用于快速刷新。
- `kind == "hello"` 或缺失：正常更新设备信息。
- 设备信息变化或首次出现时通知 UI；`probe` 不重复触发通知。

### 3.3 发送节奏与状态

| 行为 | 频率/超时 | 说明 |
| --- | --- | --- |
| 发送 `hello` | 每 4 秒 | 心跳，同时刷新在线状态 |
| 启动/手动刷新 | 立即 | 先发 `hello`，再发 `probe` |
| 收到 `probe` | 立即 | 回一个 `hello` |
| 退出 | 一次 | 发 `bye`，并通过 TCP 发 `bye` 帧 |
| 判定离线 | 16 秒 | 16 秒内没有新发现包且没有 TCP 连接 |
| 发现后拨号 | 每 5 秒重试一次 | 拨号超时 3 秒 |
| 握手超时 | 8 秒 | 连接建立后必须完成 `hello` 交换 |

注意事项：

- `instance` 每次启动变化，但设备 ID 不变；不要用 `instance` 做主键。
- 发现和连接是分离的：发现到了不代表 TCP 已连接；TCP 连接存在时，即使发现包暂时丢失也不会判离线。
- 隐身设备仍可能保留并接受 TCP 连接，只是发现包中写成 `offline`。

### 3.4 手动连接

当发现被网络屏蔽时，用户可以输入 `IP` 或 `IP:端口`。未写端口时使用 `42224`。手动连接走与发现相同的 TCP 握手，不依赖 UDP。

---

## 4. 聊天传输协议（TCP）

### 4.1 监听与连接

- 默认监听 `:42224`（所有网卡）。
- 配置端口被占用时 **不会** 自动换端口，聊天服务启动失败。
- 每个设备对之间维护一条 TCP 长连接。
- 连接建立后，双方先交换 `hello` 帧，再开始业务帧。
- 连接读循环没有空闲超时；写操作有 15 秒写超时。
- 每连接有 64 帧发送队列；队列满时直接关闭连接，不阻塞调用方。

### 4.2 帧信封

```json
{
  "v": 1,
  "type": "chat",
  "from": "u_alice",
  "to": "u_bob",
  "id": "f_optional",
  "time": 1763096430123,
  "data": { }
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `v` | int | 是 | 协议版本，必须为 `1` |
| `type` | string | 是 | 帧类型，见下表 |
| `from` | string | 是 | 发送方设备 ID |
| `to` | string | 否 | 保留；当前实现不解析 |
| `id` | string | 否 | 保留；当前实现不解析 |
| `time` | int64 | 否 | 保留；Unix 毫秒；当前实现不解析 |
| `data` | object | 视类型 | 负载；`ping`/`pong`/`bye` 可为空 |

发送方 **MUST** 带 `v`、`type`、`from`；接收方 **MUST** 校验 `v`、`type` 非空。

### 4.3 握手

`hello` 是连接上的第一帧，负载是发送方的 `Peer`：

```json
{
  "v": 1,
  "type": "hello",
  "from": "u_alice",
  "data": {
    "peer": {
      "id": "u_alice",
      "name": "Alice",
      "signature": "在工位",
      "avatar": "iVBORw0KGgoAAAANSUhEUg...",
      "status": "online",
      "ip": "192.168.1.23",
      "port": 42224,
      "transferPort": 42225,
      "version": "1.4.0",
      "platform": "windows/amd64",
      "lastSeen": "2026-11-14T10:20:30.123456789+08:00"
    }
  }
}
```

流程：

```
主动方（outbound）                         被动方（inbound）
      TCP connect  --------------------------->  accept
      hello(peer)   --------------------------->  read hello
      read hello    <---------------------------  hello(peer)
      握手完成                                     握手完成
```

- 主动方连上后 **立即** 发送自己的 `hello`，然后读取对方的 `hello`。
- 被动方先读取对方的 `hello`，校验后再发送自己的 `hello`。
- 双方后续必须复用同一个 `bufio.Reader`，因为 `hello` 之后可能已经缓冲了业务帧。
- 若 8 秒内未完成握手，关闭连接。
- `peer.id` 为空：拒绝。
- `peer.port` 为 0：按 `42224` 处理。
- `peer.status` 为空：按 `online` 处理。
- 接收方 **MUST** 用 TCP 连接的远端地址覆盖 `peer.ip`，不要信任载荷里的 IP。
- 收到自己的 ID（连到自己）：关闭连接。

`Peer` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 设备 ID |
| `name` | string | 用户名 |
| `signature` | string | 签名 |
| `avatar` | string | base64 编码的 PNG（无 `data:` 前缀），可为空 |
| `status` | string | 在线状态 |
| `ip` | string | 对方 IP；接收方应覆盖为 TCP 源地址 |
| `port` | int | 聊天端口 |
| `transferPort` | int | TCP 附件端口；0 表示不支持 |
| `version` | string | 应用版本 |
| `platform` | string | 平台标识 |
| `lastSeen` | string | RFC 3339 时间 |
| `connected` | bool | 仅本地状态，不序列化 |
| `alias` | string | 仅本地备注，不应信任/传播 |
| `manual` | bool | 仅本地手动添加标记 |

### 4.4 同时拨号冲突

两端可能同时向对方拨号。为避免重复连接，双方用设备 ID 字典序决定保留哪一条：

- **设备 ID 较小的一方保留自己的主动连接（outbound）**。
- 设备 ID 较大的一方保留对方的入站连接。
- 不符合规则的那条连接直接关闭。

最终双方使用同一条物理连接：由较小 ID 的那台设备发起。

### 4.5 帧类型

| `type` | 方向 | `data` | 作用 |
| --- | --- | --- | --- |
| `hello` | 双向，首帧 | `Hello` | 交换设备资料 |
| `presence` | 广播 | `PresenceUpdate` | 名称、签名、头像、状态变化 |
| `bye` | 单播 | 可选 `{"reason": "quit"}` | 主动退出 |
| `ping` | 双向 | 无 | 保活探测 |
| `pong` | 双向 | 无 | 保活响应 |
| `chat` | 单播 | `Chat` | 一条聊天消息 |
| `ack` | 单播 | `Ack` | 确认收到消息 |
| `read` | 单播 | `Read` | 已读到某个时间点 |
| `typing` | 单播 | `Typing` | 正在输入 |
| `recall` | 单播/群发 | `Recall` | 撤回消息 |
| `group` | 群发 | `GroupAction` | 群邀请、更新、退出、解散 |

---

## 5. 各帧负载

### 5.1 `hello`

```json
{"v":1,"type":"hello","from":"u_a","data":{"peer":{ "...": "见 4.3" }}}
```

### 5.2 `presence`

```json
{
  "v": 1,
  "type": "presence",
  "from": "u_alice",
  "data": {
    "name": "Alice",
    "signature": "开会中",
    "avatar": "iVBORw0KGgo...",
    "status": "busy"
  }
}
```

接收方更新对应设备的名称、签名、头像、状态和最后在线时间。状态值：

`online`、`away`、`busy`、`dnd`、`invisible`、`offline`。

> 发现包会把 `invisible` 转成 `offline`，但 `presence` 帧可能直接携带 `invisible`。UI 应按不可用处理。

### 5.3 `bye`

```json
{"v":1,"type":"bye","from":"u_alice","data":{"reason":"quit"}}
```

接收方把该设备标记离线并关闭连接。当前发送方在退出时会同时发 UDP `bye` 和 TCP `bye`。

### 5.4 `ping` / `pong`

```jsonl
{"v":1,"type":"ping","from":"u_alice"}
{"v":1,"type":"pong","from":"u_bob"}
```

收到 `ping` 时回 `pong`。当前实现没有主动定时发送 `ping` 的保活循环；实现方可以自行增加，但必须容忍旧客户端不回。

### 5.5 `chat`

```json
{
  "v": 1,
  "type": "chat",
  "from": "u_alice",
  "data": {
    "message": {
      "id": "m_7f3a...",
      "convId": "u_bob",
      "senderId": "u_alice",
      "senderName": "Alice",
      "kind": "text",
      "text": "你好\nzchat",
      "sentAt": "2026-11-14T10:20:30.123456789+08:00",
      "status": "sent"
    }
  }
}
```

#### 5.5.1 `Message`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | 消息 ID，全局唯一；接收方对空值可自行生成 |
| `convId` | string | 是 | 会话 ID；**一对一消息接收方应改写为发送者 ID** |
| `senderId` | string | 是 | 发送者设备 ID；为空时用帧的 `from` |
| `senderName` | string | 否 | 发送者显示名 |
| `kind` | string | 是 | `text` / `image` / `file` / `system` |
| `text` | string | 否 | 文本内容；图片/文件可为空 |
| `file` | object | 否 | 附件元数据，见 5.5.2 |
| `sentAt` | string | 是 | RFC 3339 时间；为零时接收方按当前时间处理 |
| `editedAt` | string | 否 | 保留字段，当前无编辑功能 |
| `status` | string | 否 | `sending` / `sent` / `delivered` / `read` / `failed`，仅本地提示 |
| `delivered` | bool | 否 | 本地状态，不应作为线上语义 |
| `read` | bool | 否 | 本地状态，不应作为线上语义 |
| `recalled` | bool | 否 | 本地状态；撤回通过 `recall` 帧传播 |
| `mine` | bool | 否 | 不序列化，仅本地视图 |

说明：

- `status`、`delivered`、`read` 是发送方的本地展示状态，接收方不应依赖；真正的送达/已读由 `ack` 和 `read` 帧表达。
- `editedAt`、`recalled` 属于预留/本地字段，跨语言实现可忽略。
- 发送前发送方会清空 `file.path`；接收方也必须忽略并清空 `file.path`。

#### 5.5.2 `File`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 文件名（不含路径） |
| `size` | int64 | 是 | 字节数 |
| `mime` | string | 否 | MIME 类型；缺失时按 `application/octet-stream` |
| `url` | string | 否 | HTTP 回退下载地址 |
| `token` | string | 否 | TCP 附件令牌 |
| `path` | string | 否 | **仅本地路径，禁止发送** |
| `width` | int | 否 | 图片像素宽 |
| `height` | int | 否 | 图片像素高 |

下载策略（接收方）：

1. 若 `token != ""` 且能通过发现/握手拿到发送方 `transferPort != 0`，优先走 TCP。
2. TCP 失败后若 `url != ""`，改用 HTTP。
3. 二者都没有，或发送方离线：无法下载。
4. 图片可自动下载；普通文件通常由用户点击后下载。

#### 5.5.3 文本消息示例

```json
{
  "v": 1,
  "type": "chat",
  "from": "u_alice",
  "data": {
    "message": {
      "id": "m_1",
      "convId": "u_bob",
      "senderId": "u_alice",
      "senderName": "Alice",
      "kind": "text",
      "text": "第一行\n第二行",
      "sentAt": "2026-11-14T10:20:30.123+08:00",
      "status": "sent"
    }
  }
}
```

文本是纯 UTF-8 字符串，没有 Markdown、HTML 或富文本语义；换行原样保留。当前 UI 在发送前会去掉首尾空白，但协议本身不做限制。

#### 5.5.4 图片消息示例

```json
{
  "v": 1,
  "type": "chat",
  "from": "u_alice",
  "data": {
    "message": {
      "id": "m_2",
      "convId": "u_bob",
      "senderId": "u_alice",
      "senderName": "Alice",
      "kind": "image",
      "file": {
        "name": "photo.png",
        "size": 184320,
        "mime": "image/png",
        "token": "t_2f9c...",
        "url": "http://192.168.1.23:42230/t/8a1c.../photo.png",
        "width": 1280,
        "height": 720
      },
      "sentAt": "2026-11-14T10:21:00Z"
    }
  }
}
```

图片不是内联 base64，也不走聊天帧正文；它就是 `kind=image` 的附件消息。`width`/`height` 只用于气泡布局，缺省时接收方可使用默认尺寸。图片完整内容在附件下载完成后由接收方本地解码/缓存。

#### 5.5.6 接收方规范化

参考实现收到 `chat` 后会做以下归一化，跨语言实现建议保持一致：

| 条件 | 处理 |
| --- | --- |
| `senderId == ""` | 使用帧的 `from` |
| `id == ""` | 由接收方生成唯一消息 ID |
| `kind == ""` | 按 `text` 处理 |
| `senderName == ""` | 使用设备显示名 |
| `sentAt` 为零时间 | 使用接收方当前时间 |
| 一对一消息 | 把 `convId` 改写为 `senderId`，并按 `senderId` 查找/创建会话 |
| 群消息但本地无该群 | 丢弃该消息（等待 `group` 邀请先到） |
| `file.path` | 清空，永不使用发送方路径 |
| 重复的消息 ID | 参考实现不去重；网关/多端实现 **SHOULD** 按 `id` 去重 |

#### 5.5.5 文件消息示例

```json
{
  "v": 1,
  "type": "chat",
  "from": "u_alice",
  "data": {
    "message": {
      "id": "m_3",
      "convId": "u_bob",
      "senderId": "u_alice",
      "senderName": "Alice",
      "kind": "file",
      "file": {
        "name": "report.pdf",
        "size": 5242880,
        "mime": "application/pdf",
        "token": "t_9b02...",
        "url": "http://192.168.1.23:42230/t/c7d3.../report.pdf"
      },
      "sentAt": "2026-11-14T10:22:00Z"
    }
  }
}
```

### 5.6 `ack`

```json
{
  "v": 1,
  "type": "ack",
  "from": "u_bob",
  "data": {
    "messageId": "m_1",
    "time": 1763096430456
  }
}
```

接收方在消息**持久化之后**发送 `ack`。发送方收到后把对应消息标记为 `delivered`。`time` 为 Unix 毫秒。

当前实现不重传未确认的消息，也没有 ACK 超时重试；`ack` 丢失时发送方可能一直显示“已发送”。

### 5.7 `read`

```json
{
  "v": 1,
  "type": "read",
  "from": "u_bob",
  "data": {
    "convId": "u_alice",
    "time": 1763096430000
  }
}
```

`time` 是 Unix 毫秒水位：所有 `sentAt <= time` 的己方消息标记为已读。

- 当前发送端只为**一对一**会话主动发送 `read`，每个会话最多每秒一次。
- 接收端如果收到群会话的 `read`，也会按 `convId` 处理；实现方可以支持群已读，但不要假设对端一定发送。
- 一对一会话的 `convId` 在接收方应按“发送者 ID”解析。

### 5.8 `typing`

```json
{
  "v": 1,
  "type": "typing",
  "from": "u_alice",
  "data": {
    "convId": "u_bob",
    "on": true
  }
}
```

- 瞬时状态，不进入离线队列，不持久化。
- 发送端最多每 2 秒发一次 `on=true`。
- 接收端应在最后一次 `on=true` 后约 3–4 秒自动过期。
- 没有显式的时间戳或过期字段。
- 一对一消息的 `convId` 在接收方应按 `from` 解析；`convId` 不可信。

### 5.9 `recall`

```json
{
  "v": 1,
  "type": "recall",
  "from": "u_alice",
  "data": {
    "messageId": "m_1"
  }
}
```

- 发送端 UI 限制：消息发送后 2 分钟内才能撤回；协议本身不携带该限制。
- 接收方收到后把对应消息标记为已撤回，并停止提供其附件。
- 接收方只应在消息发送者与 `from` 一致时应用（当前实现还允许本地发送者）。
- 群聊中由发送者向每个成员单播。

### 5.10 `group`

```json
{
  "v": 1,
  "type": "group",
  "from": "u_owner",
  "data": {
    "action": "invite",
    "group": {
      "id": "g_4a7d...",
      "name": "项目组",
      "owner": "u_owner",
      "members": ["u_owner", "u_bob", "u_carol"],
      "createdAt": "2026-11-14T09:00:00Z",
      "updatedAt": "2026-11-14T09:00:00Z"
    },
    "intro": "欢迎加入"
  }
}
```

`action` 取值：

| 值 | 含义 | 接收方行为 |
| --- | --- | --- |
| `invite` | 邀请加入群 | 若成员列表包含自己，创建或更新群会话 |
| `update` | 群资料/成员变更 | 整体替换群名、群主、成员列表 |
| `leave` | 某成员退出 | 从 `members` 中移除帧的 `from` |
| `disband` | 群解散 | 删除本地群会话和消息 |

约束与注意：

- `group.id` 必须以 `g_` 开头，否则丢弃。
- 群消息由发送者分别单播给每个成员，不存在服务器转发。
- 群消息到达时如果本地还没有对应群会话，当前实现会**丢弃**该消息。因此邀请必须先到。
- 群更新是“整体覆盖”，没有版本号/epoch；并发修改以最后到达者为准。
- 当前实现没有群主权限校验，任何成员都可以发 `update` 或 `disband`。安全要求高的实现应自行加校验。

---

## 6. 聊天业务语义

### 6.1 会话 ID 与路由

**一对一：**

- 本机把对方设备 ID 直接作为会话 ID。
- 发送方消息的 `convId` 是对方设备 ID。
- 接收方无论消息里的 `convId` 是什么，都按 `senderId`（缺省为 `from`）找到/创建一对一会话，并把 `convId` 改写为发送者 ID。
- 因此设备改名、换 IP 不影响会话；跨语言实现不要用 `convId` 做一对一消息的强关联。

**群聊：**

- 群 ID 以 `g_` 开头，由创建者生成。
- 会话 ID 就是群 ID，所有成员一致。
- 群消息的 `convId` 必须是群 ID。
- 群描述通过 `group` 帧同步，消息本身不携带成员列表。

### 6.2 发送流程（文本）

```
用户输入
  -> 创建 Message{id, convId, senderId, senderName, kind:"text", text, sentAt, status:"sent"}
  -> 构造 Frame{type:"chat", data:{message}}
  -> 对每个目标（一对一 1 个；群聊 N 个成员）单播
  -> 连接可用则写发送队列；不可用则写入本地 outbox
  -> 接收方持久化后回 ack
  -> 发送方标记 delivered
```

### 6.3 离线队列与可靠性

- `chat`、`group`、`recall` 帧在目标离线时进入本地 outbox，每个目标最多 **300 条**，超出丢弃最旧的。
- 目标重新连接后按顺序补发。
- `typing`、`read`、`ack` 不进入离线队列。
- outbox 写入本地 `state.json`，重启后保留。
- 参考实现的可靠性是“尽力而为”：
  - 帧写入发送队列成功即从 outbox 删除；如果连接随后断开，该帧可能丢失。
  - `ack` 丢失不会触发重传。
  - 接收方没有按消息 ID 去重。
- 跨语言实现建议：
  - 用 `message.id` 去重，避免网关/重连造成重复展示。
  - 如需要更强可靠性，可自行增加 ACK 超时重传，但必须配合去重。

### 6.4 已读与输入状态

- 已读是**时间水位**，不是逐条确认；只覆盖 `sentAt <= read.time` 的己方消息。
- 当前只为一对一主动发已读，且节流 1 秒。
- 输入状态是瞬时提示；发送端节流 2 秒，接收端 3–4 秒过期。
- 二者都不持久化，离线期间不会补发。
- 已读水位依赖双方系统时钟大致同步；时钟偏差过大时可能误标已读/未读。

### 6.5 系统消息

`kind=system` 用于本地提示（如“你创建了群聊”“某某退出了群聊”）。

- 当前实现只在本地生成，不通过 `chat` 帧发送。
- 接收方如果收到 `kind=system`，可以显示为系统提示，也可以忽略。
- 系统消息不参与未读/通知的业务语义（当前实现收到后直接返回，但仍会回 ACK）。

### 6.6 图片流程

发送：

1. 图片先作为本地文件落到磁盘（选择文件、粘贴剪贴板、截图等）。
2. 通过附件服务登记，得到 TCP token 和可选 HTTP URL。
3. 根据 MIME 是否以 `image/` 开头设置 `kind=image`，并读取宽高。
4. 发送 `chat` 帧，正文为空，`file` 携带元数据。
5. 接收方收到消息后可自动下载；下载完成后设置本地 `file.path`，解码显示。

接收：

- `file.path` 永远不使用发送方的值。
- 图片缓存目录使用消息 ID 的 SHA-256 作为文件名主干，避免路径注入。
- 若 `width/height` 缺失，使用默认气泡尺寸；不要让缺失元数据阻止下载。

> 图片和普通文件共用完全相同的传输协议，区别只在 `kind`、MIME、宽高和接收端的自动下载策略。

---

## 7. TCP 附件传输协议

### 7.1 角色

- **发送方**运行附件服务，监听默认 `42225`（或实际协商到的端口）。
- 发送方在发现包和 `hello.peer.transferPort` 中公布该端口。
- **接收方**主动连接发送方，发送令牌和偏移量。
- 只有发送方需要入站端口；接收方只需能出站连接。
- 每个文件、每次请求使用一条新的 TCP 连接。

### 7.2 请求与响应

请求是一行 JSON（最大 16 KiB）：

```json
{"token":"t_2f9c...","offset":12345}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `token` | string | 是 | 文件令牌 |
| `offset` | int64 | 是 | 从文件第几个字节开始；0 表示从头 |

响应也是一行 JSON（最大 16 KiB）：

成功：

```json
{"ok":true,"size":5242880,"name":"report.pdf","mime":"application/pdf"}
```

失败：

```json
{"ok":false,"code":"not_found","error":"文件不存在或发送方已重启"}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ok` | bool | 是否成功 |
| `size` | int64 | 文件总大小；成功且非零时返回，缺失按 0 处理（零字节文件会省略） |
| `name` | string | 文件名 |
| `mime` | string | MIME 类型 |
| `code` | string | 稳定的机器可读错误码，见下表；旧发送方可能省略 |
| `error` | string | 失败原因；**人类可读文本，接收方不应解析** |

稳定错误码：

| `code` | 含义 | 典型触发 |
| --- | --- | --- |
| `bad_request` | 请求行不是合法 JSON | `{"token":` |
| `not_found` | 令牌不存在或已被清理 | 消息已删除、发送方重启且未持久化 |
| `file_missing` | 令牌存在但文件被移动/删除 | 用户手动移走下载源文件 |
| `bad_offset` | `offset < 0` 或 `offset > size` | `.part` 损坏、消息大小错误 |
| `file_changed` | 文件内容/大小与上次请求不一致 | 预留：续传期间文件被替换 |
| `io_error` | 发送方打开、定位或读取失败 | 权限不足、磁盘错误 |
| `too_large` | 超过声明的上限 | 预留：服务端限制单文件大小时 |
| `busy` | 并发/限流拒绝 | 预留：服务端限制同时传输数时 |
| `checksum_mismatch` | 内容校验失败 | 接收方启用 SHA-256 时本地判定 |

接收方处理建议：

- **优先按 `code` 分支**，`error` 只用于展示或日志。
- `code` 为空说明对端是旧版本，此时可退回按 `ok`/连接状态处理，并把 `error` 原样展示。
- `not_found`、`file_missing` 不应无限重试；`io_error`、`busy` 可以退避后重试一次。
- `bad_offset` 应删除本地 `.part` 并从 0 重新开始。
- 新增错误码必须只增不改；旧接收方应把未知 `code` 当作通用失败处理。

响应之后：

- 成功：服务器从 `offset` 开始，把文件剩余字节原样写入同一连接，写完后关闭。
- 失败：服务器写完错误行后关闭，不发送文件内容。
- 没有分块长度、没有每块校验、没有压缩、没有加密。
- 接收方根据 `size` 和已写字节数判断是否完成。

### 7.3 完整时序

```
接收方                                              发送方
  | 1. 从 chat 消息拿到 file.token / size              |
  |    从 peer.transferPort 拿到地址                   |
  |                                                    |
  | 2. TCP connect senderIP:transferPort               |
  |--------------------------------------------------->|
  | 3. {"token":"t_...","offset":0}\n                  |
  |--------------------------------------------------->|
  | 4. {"ok":true,"size":N,"name":"...","mime":"..."}\n|
  |<---------------------------------------------------|
  | 5. 原始文件字节（N 字节）                          |
  |<---------------------------------------------------|
  | 6. 发送方关闭连接                                   |
  | 7. 接收方校验已写字节 == size                        |
```

### 7.4 断点续传

续传不需要额外控制帧：

1. 接收方把未完成内容写入 `.part` 文件。
2. 下次下载时读取 `.part` 当前大小作为 `offset`。
3. 发送方校验 `0 <= offset <= size`，`f.Seek(offset)` 后继续发送。
4. 接收方追加写入，直到累计字节数等于 `size`。
5. 完成后把 `.part` 重命名/移动到最终路径。

边界：

- `offset == size`：服务器返回成功，随后发送 0 字节；接收方可以直接完成。
- `offset > size` 或 `offset < 0`：服务器返回错误。
- 发送方文件在两次请求之间被修改：协议没有 mtime/ETag/hash 校验，可能得到不一致内容。接收方只能靠最终大小做弱校验。
- 参考实现只校验最终大小，没有 SHA-256。

### 7.5 时限与缓冲

| 项 | 值 | 说明 |
| --- | --- | --- |
| 连接超时 | 6 秒 | 接收方拨号 |
| 请求/响应超时 | 8 秒 | 从连接到读完响应；之后取消超时 |
| 文件流空闲超时 | 无 | 依赖 TCP/系统超时 |
| 控制行上限 | 16 KiB | 请求和响应 |
| 拷贝缓冲 | 256 KiB | 发送/接收 |
| 进度上报 | 约 100 ms 节流 | 发送方；接收方约 120 ms |

### 7.6 错误条件

服务器在以下情况返回 `ok:false`：

| 条件 | 当前错误文本（中文环境） |
| --- | --- |
| 请求行不是合法 JSON | `请求格式错误` |
| 令牌不存在 | `文件不存在或发送方已重启` |
| 文件已被移动/删除 | `文件已被移动或删除` |
| `offset < 0` 或 `offset > size` | `续传位置无效` |
| 打开文件失败 | `无法读取文件` |
| seek 失败 | `无法定位文件` |

这些文本会随界面语言变化，**不能**作为程序判断依据。实现方应只看 `ok` 和 `error` 是否存在。

### 7.7 令牌生命周期

- 发送文件时为每个文件生成独立 TCP 令牌。
- 令牌映射持久化到发送方数据目录的 `transfers-tcp.json`（`token -> {name,size,mime,path,modTime}`）。
- 发送方重启后，只要文件仍存在，旧令牌继续可用。
- 删除消息、清空记录或撤回时，应用会从发送方服务移除对应令牌。
- 令牌是 bearer token；知道令牌即可下载，不校验请求方身份。

---

## 8. HTTP 附件回退与共享

### 8.1 附件 URL

发送方同时登记 HTTP 令牌时，消息的 `file.url` 形如：

```
http://<sender-ip>:<sharePort>/t/<http-token>/<url-encoded-name>
```

- `<http-token>` 是 32 位十六进制，和 TCP 的 `t_...` 令牌**不是同一个值**。
- 路径里的文件名只用于展示/下载名，服务器按 token 查找，不按文件名查找。
- 主机通常是发送方主网卡 IPv4；接收方若不可达，可尝试用已知 peer IP 替换主机部分（非规范强制，但便于容错）。

### 8.2 下载语义

```
GET /t/<token>/<name>
Range: bytes=<offset>-
```

- 支持标准 HTTP `Range`；参考客户端使用开放式单区间 `bytes=N-`。
- 服务器通过 `http.ServeContent` 提供：
  - 首次请求通常 `200 OK`；
  - 带 Range 时 `206 Partial Content`；
  - 越界时 `416 Requested Range Not Satisfiable`。
- 若服务器忽略 Range 返回 `200`，接收方必须丢弃已有 `.part` 并从 0 重写，避免拼接错位。
- 若 `416` 且本地偏移已等于消息中的 `size`，可视为已完成。
- 令牌不存在或文件消失：`404`，并在响应头带 `X-Zchat-Error-Code: not_found` 或 `file_missing`。
- 接收方应优先读取 `X-Zchat-Error-Code`，避免依赖本地化正文；旧服务器没有该头时按状态码处理。
- 可选 `?download=1`：增加 `Content-Disposition: attachment`，便于浏览器直接下载。
- 响应 `Content-Type` 来自登记的 MIME。

### 8.3 其他 HTTP 端点

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/t/<token>/<name>` | 下载消息附件 |
| GET | `/health` | 返回纯文本 `zchat\n`，用于探活 |
| GET | `/` | 共享目录的 HTML 列表，支持 `?q=` 搜索 |
| GET | `/<相对路径>` | 下载共享目录中的文件；`?download=1` 强制附件 |
| POST | `/upload` | multipart/form-data 上传；需要发送方显式开启上传目录 |

上传约束：

- 仅当发送方明确开启“上传目录”且服务由用户显式启动时可用；普通附件回退服务是只读的。
- 单次上传上限 **8 GiB**（`8 << 30` 字节）。
- 表单字段 `files` 可多个；`next` 指定上传完成后跳回的站内路径。
- 成功后 `303 See Other` 重定向到 `next?uploaded=N`。
- 文件名会做路径分隔符、控制字符、Windows 保留名、长度等清理；同名文件自动加序号。

### 8.4 可用性

- TCP 附件服务在应用启动时就会启动，旧消息的 TCP 令牌通常可用。
- HTTP 服务只有在“启动时共享”开启，或本次运行发送过附件时才会启动。
- 因此发送方重启后，旧消息的 `file.url` 可能暂时不可用，但 `file.token` 仍可能可用。
- 接收方应先 TCP、后 HTTP；不要因为 URL 存在就跳过 TCP。

---

## 9. 限制、故障与安全

### 9.1 主要限制

| 项 | 限制 |
| --- | --- |
| 聊天帧 | 2 MiB |
| 附件控制行 | 16 KiB |
| 发现包接收缓冲 | 4096 字节 |
| 发送队列 | 每连接 64 帧，满则断连 |
| 离线队列 | 每设备 300 条 |
| HTTP 上传 | 8 GiB/次 |
| 附件大小 | 协议使用 int64，应用未设上限 |
| IPv6 | 发现为 IPv4；手动连接地址格式未完整支持 IPv6 |
| 加密/认证 | 无 |

### 9.2 常见故障定位

| 现象 | 可能原因 |
| --- | --- |
| 发现不到设备 | 组播/广播被屏蔽；UDP 42223 被占用；防火墙；不同子网 |
| 能发现但连不上 | TCP 42224 被防火墙拦截；端口配置不一致；手动地址错误 |
| 文本能发、文件不能 | `transferPort=0`；附件端口被防火墙拦截；发送方文件已删除 |
| 旧消息文件下载失败 | 发送方重启后 HTTP 共享未启动；令牌被删除；文件被移动 |
| 收到重复消息 | 网关/重连重复投递；参考实现无去重，需按消息 ID 去重 |
| 一直显示“已发送” | ACK 丢失或对端未持久化；参考实现不会重传 |

### 9.3 安全建议

- 仅在可信局域网或 VPN 内使用。
- 不要将 42224/42225/42230 暴露到公网。
- 附件和共享目录没有身份校验；令牌泄露等于文件泄露。
- 上传目录应使用独立目录并限制权限；不需要时关闭。
- 如果跨语言实现用于不可信网络，应在协议外增加 TLS、设备配对、消息签名和重放保护。

---

## 10. 跨语言实现清单

### 10.1 最小可用实现（只支持文本）

1. 监听 TCP，默认 42224。
2. 实现 `hello` 握手：主动方先发，被动方读后回。
3. 按行读取 JSON，校验 `v==1`、`type` 非空、帧不超 2 MiB。
4. 实现 `chat`（`kind=text`）和 `ack`。
5. 一对一接收时用 `senderId` 作为会话 ID。
6. 忽略未知字段、未知类型和未知 kind。
7. 发送 `\n` 分帧，UTF-8 编码。

### 10.2 完整实现（文本 + 图片 + 文件）

在最小实现基础上：

1. 实现 UDP 发现：组播 + 广播，4 秒心跳，16 秒超时，处理 `hello`/`probe`/`bye`。
2. 实现 `presence`、`read`、`typing`、`recall`、`group`。
3. 实现离线 outbox（至少 chat/group/recall），每目标 300 条。
4. 启动 TCP 附件服务；在发现包和 `hello.peer.transferPort` 中公布端口。
5. 发送附件时生成 token，登记文件，构造 `file` 元数据。
6. 接收时优先 TCP 拉取，按 `.part` 大小续传，完成后校验大小。
7. 可选实现 HTTP 回退：`/t/<token>/<name>` + Range。
8. 图片按 `kind=image` + `mime=image/*` 处理；缺宽高时使用默认布局。
9. 对消息按 `id` 去重；对群消息确认群会话已存在。

### 10.3 兼容性注意事项

- **不要**把 `file.path` 发给对端，也不要用对端的 `file.path`。
- **不要**解析本地化错误文本判断错误类型。
- **不要**假设 `convId` 在一对一接收端等于发送方使用的值。
- **不要**假设群消息一定能在邀请之前到达。
- **不要**假设 ACK 一定到达或一定只到达一次。
- **不要**假设 `ping` 会周期性出现；也不要因为没收到 `ping` 就断开。
- **不要**假设头像一定是 PNG；按 base64 图片解码失败时回退到文字头像。
- 新增字段只增不改；改变语义时必须提升协议版本。

### 10.4 建议的接收状态机

```
收到帧
  ├─ JSON 解析失败 / v != 1 / type 为空 -> 关闭连接
  ├─ type == hello     -> 校验 peer，注册连接
  ├─ type == presence  -> 更新 peer
  ├─ type == bye       -> 标记离线，关闭连接
  ├─ type == ping      -> 回 pong
  ├─ type == pong      -> 忽略
  ├─ type == chat      -> 去重 -> 持久化 -> 回 ack -> 路由到会话
  ├─ type == ack       -> 标记消息 delivered
  ├─ type == read      -> 按时间水位标记已读
  ├─ type == typing    -> 更新临时状态，3–4 秒过期
  ├─ type == recall    -> 校验发送者 -> 标记撤回
  ├─ type == group     -> 校验 g_ 前缀 -> 应用群操作
  └─ 其他              -> 忽略
```

---

## 附录 A：类型速查（TypeScript 风格）

以下定义只描述线上 JSON，便于生成各语言类型；Go 结构体仍是权威实现。

```ts
type Presence = "online" | "away" | "busy" | "dnd" | "invisible" | "offline";
type MessageKind = "text" | "image" | "file" | "system";
type MessageStatus = "sending" | "sent" | "delivered" | "read" | "failed";

type FrameType =
  | "hello" | "presence" | "bye" | "ping" | "pong"
  | "chat" | "ack" | "read" | "typing" | "recall" | "group";

interface Frame<T = unknown> {
  v: 1;
  type: FrameType | string; // 未知类型应忽略
  from: string;
  to?: string;              // 保留
  id?: string;              // 保留
  time?: number;            // 保留，Unix 毫秒
  data?: T;
}

interface Peer {
  id: string;
  name: string;
  signature?: string;
  avatar?: string;          // base64 PNG，无 data: 前缀
  status: Presence;
  ip?: string;
  port?: number;
  transferPort?: number;
  version?: string;
  platform?: string;
  lastSeen: string;         // RFC 3339
}

interface FileMeta {
  name: string;
  size: number;
  mime?: string;
  url?: string;
  token?: string;
  width?: number;
  height?: number;
  // path 仅本地使用，不进入线上 JSON
}

interface Message {
  id: string;
  convId: string;
  senderId: string;
  senderName?: string;
  kind: MessageKind;
  text?: string;
  file?: FileMeta;
  sentAt: string;           // RFC 3339
  editedAt?: string;
  status?: MessageStatus;
  delivered?: boolean;
  read?: boolean;
  recalled?: boolean;
}

interface Hello { peer: Peer; }
interface PresenceUpdate { name: string; signature?: string; avatar?: string; status: Presence; }
interface Chat { message: Message; }
interface Ack { messageId: string; time: number; }
interface Read { convId: string; time: number; }
interface Typing { convId: string; on: boolean; }
interface Recall { messageId: string; }
interface GroupAction {
  action: "invite" | "update" | "leave" | "disband";
  group: { id: string; name: string; owner: string; members: string[]; createdAt: string; updatedAt: string; };
  intro?: string;
}

interface Announcement {
  app: "zchat";
  v: 1;
  kind: "hello" | "probe" | "bye";
  id: string;
  instance?: string;
  name: string;
  signature?: string;
  status: Presence;
  version?: string;
  platform?: string;
  port: number;
  transferPort?: number;
  ts: number;               // Unix 毫秒
}

interface TransferRequest { token: string; offset: number; }
interface TransferResponse {
  ok: boolean;
  size?: number;
  name?: string;
  mime?: string;
  code?: string;            // 稳定错误码，见 7.2；旧端可能省略
  error?: string;           // 人类可读，勿用于程序判断
}
```

---

## 11. 推荐扩展与优化

以下都是**可选增量**，不改变当前 `v=1` 的必填字段和既有语义。旧实现会忽略未知字段；新实现必须在字段缺失时保持原行为。按投入产出比排序：

| 优先级 | 建议 | 主要收益 | 兼容方式 |
| --- | --- | --- | --- |
| P0 | `sha256` 内容校验 | 发现损坏/被替换，避免“大小对但内容错” | 可选字段；缺失则跳过校验 |
| P0 | 稳定错误码 `code` | 跨语言程序可判断失败原因 | 保留 `error` 文本；旧端忽略 `code` |
| P0 | `mtime` / 版本一致性 | 续传期间文件被替换时能发现 | 可选请求/响应字段 |
| P1 | `caps` 能力与限制声明 | 新功能灰度、避免发送必然失败的大文件 | 未知能力忽略 |
| P1 | 消息去重 + 有界重传 | 降低“已发送但未送达”概率 | 双方声明能力后启用 |
| P1 | 群状态 `revision` | 避免旧群更新覆盖新状态 | 缺失时回退到当前行为 |
| P1 | HTTP `ETag` / `If-Range` | 回退续传更安全，避免拼接旧内容 | 标准 HTTP，旧客户端无感 |
| P2 | 批量 `batchId` | 多文件进度聚合展示 | 纯展示字段，不引入会话 |
| P2 | 可选安全层 | 用于不完全可信网络 | 建议作为 v2，不塞进 v1 |

### 11.1 P0：附件内容校验 `sha256`

在 `File` 和 TCP 附件响应中增加可选 `sha256`（小写十六进制，SHA-256）：

```json
{
  "name": "report.pdf",
  "size": 5242880,
  "mime": "application/pdf",
  "token": "t_9b02...",
  "sha256": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
}
```

```json
{"ok":true,"size":5242880,"name":"report.pdf","mime":"application/pdf","sha256":"9f86d081..."}
```

建议：

- 发送方登记文件时计算一次并缓存，不要每次请求重读全文件。
- 接收方在**完整下载并校验大小之后**再校验哈希；不匹配则删除 `.part`，返回 `code: "checksum_mismatch"`。
- 旧发送方没有 `sha256` 时，接收方跳过校验，不能因此拒绝文件。
- HTTP 回退可在响应头增加 `X-Zchat-Sha256`，或继续使用消息里的 `file.sha256`。
- 哈希只解决完整性，不解决身份认证和加密。

### 11.2 P0：稳定错误码 `code`（已实现）

TCP 附件响应已带可选 `code`，程序应只依赖它，不解析本地化 `error`：

```json
{"ok":false,"code":"bad_offset","error":"续传位置无效"}
```

当前实现返回的错误码：

| `code` | 含义 | 状态 |
| --- | --- | --- |
| `bad_request` | 请求行不是合法 JSON | 已实现 |
| `not_found` | 令牌不存在或已失效 | 已实现 |
| `file_missing` | 令牌存在但文件被移动/删除 | 已实现 |
| `bad_offset` | `offset < 0` 或 `offset > size` | 已实现 |
| `io_error` | 打开、seek 或读取失败 | 已实现 |
| `file_changed` | 文件大小/`mtime` 与预期不一致 | 预留（配合 11.3） |
| `checksum_mismatch` | 内容校验失败 | 预留（配合 11.1） |
| `too_large` | 超过声明的文件大小上限 | 预留（启用限流/限额时） |
| `busy` | 并发/限流拒绝 | 预留（启用并发上限时） |

HTTP 回退在 `404` 时带响应头 `X-Zchat-Error-Code: not_found` 或 `file_missing`，正文和状态码保持原样；旧客户端只看到原来的 `404`。

兼容约束：

- `code` 只增不改；旧发送方没有 `code` 时，接收方按 `error` 或连接状态兜底。
- 接收方遇到未知 `code` 必须当作通用失败，不能因此崩溃或无限重试。

### 11.3 P0：文件版本一致性

当前续传只校验 `offset <= size`。如果发送方文件在两次请求之间被替换，接收方可能把新旧内容拼在一起。建议：

- TCP 响应增加 `mtime`（Unix 毫秒）和 `size`。
- 请求增加可选 `expectSize`、`expectMtime`；发送方 `stat` 后不一致直接返回 `file_changed`。

```json
{"token":"t_9b02...","offset":12345,"expectSize":5242880,"expectMtime":1763096430000}
```

```json
{"ok":true,"size":5242880,"mtime":1763096430000,"name":"report.pdf","mime":"application/pdf"}
```

接收方收到 `file_changed` 时应删除 `.part`，从 0 重新开始，而不是继续追加。发送方的 `transfers-tcp.json` 已经保存 `modTime`，无需新增存储。

HTTP 回退建议使用真实的文件 `mtime` 生成 `Last-Modified` 和 `ETag`（例如 `"<size>-<mtime>"`），客户端带 `If-Range`；内容变化时服务器返回 `200`，客户端丢弃旧 `.part` 重下。

### 11.4 P1：能力与限制声明

在发现包和 `hello.peer` 中增加可选 `caps` 与 `limits`，让新功能可以灰度，不必立即升 `v`：

```json
{
  "caps": ["tcp-transfer", "http-fallback", "sha256", "group", "ack-retry"],
  "limits": {"maxFileSize": 8589934592, "maxFrame": 2097152}
}
```

建议：

- 能力名使用稳定的短字符串；未知能力必须忽略。
- 发送前检查 `limits.maxFileSize`，避免传完才发现被拒。
- `sha256`、`mtime` 这类“发送方附带、接收方可选使用”的能力不需要双方都声明。
- `ack-retry`、多文件会话这类改变交互状态的能力，必须双方都声明后再启用。
- 发现包只放少量能力/数字，不要放头像、诊断信息或大对象。

### 11.5 P1：消息去重与有界重传

当前 ACK 丢失不会重传，消息可能停在“已发送”。若要增强可靠性：

1. 接收方持久化最近消息的 `(senderId, message.id)` 去重集合（可按 7 天或每会话上限清理）；重复消息只回 ACK，不重复入库。
2. 发送方保留待确认的 `chat`，在 5s / 15s / 45s 后重试，最多 3 次；收到 ACK 或超过次数后停止。
3. `typing`、`read` 永不重传；`recall`、`group` 可重传，但接收方应幂等。
4. 只在双方 `caps` 都包含 `ack-retry` 时启用，避免旧客户端重复展示。
5. 重传必须有上限并进入现有离线队列/清理策略，避免无限增长。

这会把语义从“最多一次”变成“至少一次 + 去重”；没有去重就不要开启重传。

### 11.6 P1：群状态版本 `revision`

群操作目前是整体覆盖、最后到达者获胜，网络乱序或重连补发可能让旧状态覆盖新状态。建议给 `Group` 增加可选 `revision`（int64，群主每次修改递增）：

```json
{
  "action": "update",
  "group": {
    "id": "g_4a7d...",
    "name": "项目组",
    "owner": "u_owner",
    "members": ["u_owner", "u_bob"],
    "revision": 7,
    "createdAt": "2026-11-14T09:00:00Z",
    "updatedAt": "2026-11-14T10:00:00Z"
  }
}
```

接收方只应用 `revision > 本地 revision` 的更新；重复或更旧的更新直接忽略。旧客户端没有 `revision` 时，继续按当前“整体覆盖”处理。

可选硬化：当 `group.owner` 非空时，忽略 `from != group.owner` 的 `update` / `disband`。这是策略层校验，不改变线上格式；如果未来支持管理员，再显式增加角色字段。

### 11.7 P1：HTTP 回退的一致性

- `Last-Modified` 建议使用**文件真实 mtime**，而不是令牌创建时间；`ETag` 使用 `size + mtime`。
- 支持 `If-Range`：文件已变化时返回 `200` 全量，客户端丢弃旧 `.part`，避免新旧内容拼接。
- 附件响应增加 `Cache-Control: private, no-store`，避免令牌 URL 被中间缓存。
- 保持现有 `Range`、`416`、`?download=1` 和 `/health` 行为不变。
- 如果未来给令牌加 `expiresAt`，默认值应为“不过期”，否则会破坏旧消息的离线下载；过期只作为可选的共享场景策略。

### 11.8 P2：批量发送只做展示聚合

多文件场景不必引入服务器会话。给消息增加可选 `batchId`，同一批文件共用：

```json
{"id":"m_1","kind":"file","batchId":"b_7f3a...","file":{"name":"a.png","size":1234,"token":"t_..."}}
```

- UI 按 `batchId` 聚合进度、提供“全部取消/全部另存”。
- 传输仍按文件独立 token/连接，不改变现有模型。
- 如果确实需要“一次协商、多文件统一接收/拒绝”，那应设计新的 `transfer` 帧或会话层，并作为 v2 能力，不建议塞进 `chat`。

### 11.9 P2：可选安全层（建议作为 v2）

当前协议假设可信局域网；哈希和错误码不提供安全性。若需要面对不完全可信网络：

- **优先方案**：TLS 1.3 + 自签名证书 + 证书指纹固定（类似 LocalSend），或直接用 VPN/受控隧道。
- **应用层方案**：设备配对后共享密钥，对 `v/type/from/id/time/data` 的规范化字节做 HMAC-SHA256，帧增加 `sig`；发现包增加一次性 `nonce`/时间窗校验防重放。
- 附件令牌可增加 `expiresAt` 和绑定设备 ID；HTTP 侧避免把令牌写进日志。
- 不建议自定义加密算法或自创密钥交换；这会显著增加跨语言实现和安全审计成本。

### 11.10 P2：实现层优化（不改协议）

- 每个对端/全局限制并发下载数（例如 2–4），服务端用信号量限制同时传输，避免大量 goroutine 和磁盘争抢。
- 发送方缓存 SHA-256/mtime，避免每次请求重算；文件流保持 256 KiB 缓冲，不要整文件读入内存。
- 头像限制尺寸（例如 256×256 PNG、≤256 KiB），并按哈希缓存；`presence` 只在头像真正变化时携带，避免大 base64 反复占帧。
- 发现包保持精简（约几百字节），不要加入头像、版本日志或诊断数据。
- 定期清理：孤立的 `.part`、文件已不存在的令牌、超过保留期的去重记录；删除消息时已会移除令牌。
- 超时/重试使用单调时钟，避免系统时间跳变；展示时间才用墙钟。

### 11.11 不建议的做法

- 不要把文件正文或大图片 base64 塞进 `chat` 帧；会挤占 2 MiB 帧上限并使消息通道阻塞。
- 不要为了“标准化”直接在当前 v1 上引入中心服务器或强制会话状态；会破坏零配置和拉取模型。
- 不要在协议内自创加密；优先 TLS/VPN，或把安全层作为独立的 v2 设计。
- 不要解析 `error` 文本做程序判断；新增 `code` 后以 `code` 为准。
- 不要在没有去重的前提下开启 ACK 重传；会造成重复消息。
