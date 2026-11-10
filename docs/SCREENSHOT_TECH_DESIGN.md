# zchat 截图与贴图技术规划

- 文档版本：v0.1（规划稿）
- 日期：2026-11-08
- 状态：待评审，暂不修改代码
- 关联文档：
  - [项目规划](SCREENSHOT_PROJECT_PLAN.md)
  - [功能规划](SCREENSHOT_FEATURES.md)
  - [现有设计说明](../docs/DESIGN.md)
  - [现有构建指南](../docs/BUILD.md)

## 1. 摘要

截图能力分为四层：

1. **平台截屏层**：取得显示器、窗口和屏幕像素。
2. **截图会话层**：隐藏主窗口、等待合成器、选区、裁剪、完成动作。
3. **标注渲染层**：操作模型、预览、撤销/重做、最终 PNG 导出。
4. **应用集成层**：overlay 窗口、贴图窗口、聊天发送、设置、快捷键。

核心原则：

- 平台相关代码集中在 `internal/capture`，通过 build tag 隔离。
- 标注模型和几何逻辑不依赖 UI，可单元测试。
- `internal/annotate` 只依赖标准库和 `golang.org/x/image`。
- UI 状态仍由 `internal/app` 在主线程持有，平台截屏在工作 goroutine 执行，通过现有 `post` 回到主线程。
- 不修改 `proto`、`transfer`、`share` 的协议；发送截图复用现有图片附件链路。

## 2. MyGo 能力核查

在 MyGo v0.7.4 上确认：

| 能力 | 结论 | 用法 |
| --- | --- | --- |
| 屏幕/显示器枚举 | 有 | `mygo.Screen.Displays()`、`CursorScreenPoint()`、`DisplayNearestPoint()`、`OnDisplaysChanged()` |
| 桌面截屏 | **没有现成 API** | 必须实现 Windows/macOS/Linux 平台后端 |
| 窗口截图 | **没有桌面窗口截图 API** | `Window.CapturePage()` 只能截 zchat 自己的窗口，不能截其他应用 |
| 原生 overlay 窗口 | 可用 | `mygo.NewWindow` + `ui.View`，`Frameless`、`AlwaysOnTop`、`SkipTaskbar`、`Panel` |
| 原生绘制 | 可用 | `ui.Painter`、`ui.Bitmap`、`ui.Image`、`ui.Canvas` |
| 指针/键盘输入 | 可用 | `Element.HandleInput`、`InputEvent`、`Focus()`、`Cursor()` |
| 剪贴板图片 | 可用 | `mygo.Clipboard.ReadImage()` / `WriteImage(png []byte)` |
| 全局快捷键 | 可用 | `mygo.GlobalShortcut.Register`；Wayland 下 MyGo 可通过 XDG portal 注册 |
| 多窗口 | 可用 | 每个贴图一个 `mygo.Window` |
| 窗口透明度/鼠标穿透/跨工作区 | 可用 | `SetOpacity`、`SetIgnoreMouseEvents`、`SetVisibleOnAllWorkspaces` |
| 第二实例参数 | 可用 | `App.OnSecondInstance(func(args []string, cwd string))`，可用于 `--capture` |

结论：MyGo 能承担窗口、UI、快捷键、剪贴板和附件发送，但**桌面像素采集必须新增平台后端**。

## 3. 总体架构

```text
┌────────────────────────────────────────────────────────────┐
│ internal/app                                               │
│  截图会话 · overlay UI · 标注工具栏 · 贴图窗口 · 聊天集成   │
│  设置 · 快捷键 · 历史 · 主窗口隐藏/恢复                     │
└───────────────┬───────────────────────────┬────────────────┘
                │                           │
      ┌─────────▼─────────┐       ┌─────────▼─────────┐
      │ internal/capture  │       │ internal/annotate │
      │ 显示器/窗口/像素   │       │ 操作模型/裁剪/导出 │
      │ 平台后端          │       │ 纯 Go，无 UI 依赖  │
      └─────────┬─────────┘       └─────────┬─────────┘
                │                           │
   ┌────────────▼────────────┐    ┌─────────▼─────────┐
   │ Windows GDI / macOS CG  │    │ x/image + 标准库   │
   │ X11 XGetImage / Portal  │    │ PNG / 字体 / 光栅  │
   └─────────────────────────┘    └───────────────────┘
                │
      ┌─────────▼─────────┐
      │ 现有聊天附件链路   │
      │ sendFiles / TCP   │
      │ HTTP 回退 / 缓存   │
      └───────────────────┘
```

依赖方向：

```text
app -> capture, annotate, mygo
capture -> 标准库，平台系统库（通过 build tag）
annotate -> 标准库, golang.org/x/image
capture、annotate 不依赖 app，不依赖 mygo/ui
```

这样可以：

- 在无窗口环境测试坐标、裁剪、标注和导出；
- 用 fake backend 测试完整截图会话；
- 平台后端分别编译，不强依赖 cgo。

## 4. 包与文件规划

```text
internal/
├── capture/
│   ├── capture.go              # 类型、Backend 接口、能力、错误、会话辅助
│   ├── display.go              # 显示器模型与 DPI 映射
│   ├── window.go               # 窗口信息、命中测试、边界过滤
│   ├── encode.go               # RGBA 转换、PNG 编码辅助
│   ├── capture_windows.go      # Windows 截屏与窗口枚举
│   ├── capture_darwin.go       # macOS 截屏、权限、窗口枚举
│   ├── capture_linux.go        # 运行时选择 X11/Wayland
│   ├── capture_linux_x11.go    # X11 截屏与窗口枚举
│   ├── capture_linux_wayland.go# Portal/外部工具
│   ├── capture_other.go        # 不支持的平台
│   └── capture_test.go         # 坐标/转换/假后端测试
├── annotate/
│   ├── annotate.go             # Op、Doc、撤销/重做
│   ├── render.go               # 最终图像渲染
│   ├── text.go                 # 系统字体发现与文字绘制
│   ├── mosaic.go               # 像素化/模糊
│   └── render_test.go          # 黄金图像测试
└── app/
    ├── screenshot.go           # 状态、入口、会话生命周期
    ├── screenshot_ui.go        # overlay 视图、选区、工具栏
    ├── screenshot_tools.go     # 标注工具交互
    ├── screenshot_pin.go       # 贴图窗口
    ├── screenshot_settings.go  # 设置项、快捷键注册
    └── screenshot_files.go     # 保存、临时文件、历史
```

需要修改的现有文件（后续实现阶段）：

- `internal/app/app.go`：新增截图状态、启动时注册快捷键、退出清理、第二实例 `--capture`。
- `internal/app/ui_chat.go`：输入区截图按钮。
- `internal/app/menu.go`：文件菜单、编辑菜单快捷键。
- `internal/app/tray.go`：截图、贴图菜单。
- `internal/app/settings.go`：新增设置字段与默认值。
- `internal/app/ui_settings.go`：截图设置分组。
- `internal/app/icons.go`：截图、图钉、工具图标。
- `internal/i18n/en.tsv`：英文文案。
- `go.mod`：新增必要的纯 Go 依赖。
- `README.md` / `docs/*`：功能与构建说明（评审后更新）。

## 5. 核心类型

以下为设计草案，实际命名可在实现时调整。

```go
package capture

type Display struct {
    ID      string
    Name    string
    Bounds  image.Rectangle // DIP，主显示器左上角为 (0,0)
    Scale   float64        // 物理像素 / DIP
    Primary bool
}

type Frame struct {
    DisplayID string
    Bounds    image.Rectangle // 与 Display.Bounds 相同
    Scale     float64
    Image     *image.RGBA     // 物理像素
    CapturedAt time.Time
}

type WindowInfo struct {
    ID      uint64
    Title   string
    AppName string
    Bounds  image.Rectangle // DIP
    Layer   int
    ZOrder  int
}

type Capability struct {
    PreCaptureOverlay bool // 能否在截图前显示全屏选区
    WindowDetection   bool
    AllDisplays       bool
    Cursor            bool
    GlobalShortcut    bool
    Notes             []string
}

type Options struct {
    DisplayID    string
    IncludeCursor bool
    AllDisplays  bool
}

type Backend interface {
    Displays() ([]Display, error)
    Capabilities() Capability
    Capture(ctx context.Context, opts Options) ([]Frame, error)
    Windows(ctx context.Context, displayID string) ([]WindowInfo, error)
    Permission() Permission
    RequestPermission(ctx context.Context) Permission
}
```

错误分类：

```go
var (
    ErrPermissionDenied = errors.New("screen capture permission denied")
    ErrUnsupported      = errors.New("screen capture unsupported")
    ErrNoDisplay        = errors.New("no display")
    ErrCanceled         = errors.New("screen capture canceled")
    ErrCaptureFailed    = errors.New("screen capture failed")
    ErrTooLarge         = errors.New("screen capture too large")
)
```

`Permission` 为枚举：`Granted`、`Denied`、`NotDetermined`、`Unavailable`、`NotRequired`。

### 5.1 为什么不用 MyGo 的 `Window.CapturePage`

`CapturePage` 只截取本应用窗口自身的内容，不能读取桌面或其他窗口。它适合给贴图窗口做预览，不适用于截图功能。

## 6. 平台截屏后端

### 6.1 Windows

#### 截屏

使用 GDI：

1. `GetDC(NULL)` 取得屏幕 DC。
2. `CreateCompatibleDC`、`CreateCompatibleBitmap`。
3. `BitBlt(dst, 0, 0, w, h, screenDC, x, y, SRCCOPY|CAPTUREBLT)`。
4. `GetDIBits` 以 32 位 `BI_RGB`、负高度（自上而下）读入缓冲区。
5. 将 BGRA 转为 RGBA（alpha 置 255）。
6. 释放 `HBITMAP`、`HDC`、`ReleaseDC`。

关键点：

- 使用 `GetSystemMetrics(SM_XVIRTUALSCREEN/SM_YVIRTUALSCREEN/SM_CXVIRTUALSCREEN/SM_CYVIRTUALSCREEN)` 或显示器原生矩形。
- 以 MyGo 的 DIP 坐标定位显示器，以原生物理坐标截取。
- `CAPTUREBLT` 可以把分层窗口纳入截图；某些驱动上会带来轻微闪烁，作为可选项或按需启用。
- 默认只截当前显示器，减少内存和合成开销。
- 光标不包含在 BitBlt 中。需要时用 `GetCursorInfo` + `DrawIconEx` 在内存 DC 上补画。
- HDR/宽色域、受保护内容、DRM 窗口可能颜色异常或黑屏；首版接受限制，后续评估 Windows Graphics Capture。

#### 显示器

- `EnumDisplayMonitors` + `GetMonitorInfoW` 获取原生矩形、工作区、主显示器标记。
- 通过 MyGo `Screen.Displays()` 获取 DIP 边界和 `ScaleFactor`，用原生矩形校准。
- 原生物理矩形与 DIP 的换算：

```text
scale = nativeWidth / dipWidth
dip   = displayDIPMin + (native - nativeMin) / scale
```

- 不依赖某一个 DPI API；如果 `GetDpiForMonitor`/`GetScaleFactorForMonitor` 不可用，使用宽高比校准。

#### 窗口枚举

- `EnumWindows`，按 z-order 从上到下。
- 过滤：不可见、最小化、零尺寸、桌面/任务栏/输入法、zchat 自身进程。
- 使用 `DwmGetWindowAttribute(DWMWA_EXTENDED_FRAME_BOUNDS)` 获取无阴影边界；失败回退 `GetWindowRect`。
- 使用 `DWMWA_CLOAKED` 过滤虚拟桌面隐藏窗口。
- `GetWindowTextW` 获取标题；`GetWindowThreadProcessId` 过滤自身。
- 窗口矩形转换为 DIP，供 overlay 命中测试。

### 6.2 macOS

#### 权限

1. `CGPreflightScreenCaptureAccess()` 预检。
2. 未授权时 `CGRequestScreenCaptureAccess()` 请求。
3. 仍拒绝则返回 `ErrPermissionDenied`，由 app 显示引导。
4. 旧版 macOS 授权后可能需要重启应用，界面提示。

#### 截屏

主路径使用 CoreGraphics：

- `CGGetActiveDisplayList` / `CGGetOnlineDisplayList` 获取 `CGDirectDisplayID`。
- `CGDisplayBounds` 得到点坐标（DIP）。
- `CGDisplayPixelsWide/High` 得到 Retina 物理像素。
- `CGDisplayCreateImage(displayID)` 截取整屏。
- 需要窗口或区域时使用 `CGWindowListCreateImage`：
  - `CGRectNull`
  - `kCGWindowListOptionIncludingWindow`
  - `kCGWindowImageBoundsIgnoreFraming | kCGWindowImageBestResolution`
  - 目标 window ID。
- 将 `CGImage` 绘制到 sRGB 的 `CGBitmapContext`，统一为 8 位预乘 RGBA，再复制为 `image.RGBA`。
- 所有 `CGImageRef`、`CGDataProviderRef`、`CGColorSpaceRef`、`CFArrayRef` 都要 `CFRelease`。

回退路径：

- 如果 CoreGraphics 符号不可用、返回空图或被系统限制，调用系统 `screencapture`：
  - 全屏：`/usr/sbin/screencapture -x -t png <tmp>`
  - 区域：`-R x,y,w,h`
  - 窗口：`-l <windowID>`
  - 光标：`-C`
- 读取临时 PNG 后删除。
- 回退是兼容措施，不应成为默认主路径。

#### 窗口枚举

- `CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID)`。
- 过滤 `kCGWindowLayer == 0`、`kCGWindowAlpha > 0`、`kCGWindowOwnerPID != self`。
- `kCGWindowBounds` 是屏幕坐标字典（x/y/width/height）。
- 没有屏幕录制权限时，窗口名可能不可见；仍可用 ID + bounds，标题回退为空。
- 窗口截图优先使用 `CGWindowListCreateImage`，但首版可以只做“屏幕区域裁剪”，避免窗口被遮挡时行为不一致。

#### 已知限制

- `CGDisplayCreateImage` / `CGWindowListCreateImage` 在新 macOS 上被标记为废弃，但仍可用；后续可增加 ScreenCaptureKit 后端。
- 全屏应用、Stage Manager、多 Space 下 overlay 的置顶和跨工作区行为需要实机验证。
- 建议 overlay 使用 MyGo `Panel`，并在 `WhenReady` 后调用 `SetVisibleOnAllWorkspaces(true)`。

### 6.3 Linux X11

#### 依赖

推荐使用纯 Go 的 X11 协议库（如 `github.com/jezek/xgb` + `xproto`/`randr`/`xinerama`），避免 cgo 和 Xlib 头文件依赖。

如果不引入 X 协议库，也可用 `purego` 动态加载 `libX11.so.6`；两者都必须遵守同一坐标和像素格式模型。

#### 截屏

1. `xgb.NewConn()` 连接 `$DISPLAY`。
2. 取根窗口，必要时用 RandR/Xinerama 获取显示器布局。
3. `xproto.GetImage` 请求根窗口指定矩形：
   - `format = ZPixmap`
   - `plane_mask = ^uint32(0)`
4. 根据 `Setup().PixmapFormats`、`depth`、`bits_per_pixel`、`byte_order` 和 red/green/blue masks 转换像素。
5. 处理 16/24/32 bpp，支持常见 ARGB/BGRX 排列。
6. 将结果裁剪为当前显示器并转换为 `image.RGBA`。

注意：

- X11 的根坐标通常是物理像素；HiDPI 下 MyGo 的 DIP 可能带 `ScaleFactor`。必须通过显示器矩形和缩放比校准，不能直接把 DIP 当像素。
- 如果没有 Composite 管理器，根窗口截图仍可工作；如果某些驱动返回黑屏，回退到 `import -window root` 或 `gnome-screenshot`。
- 可选优化：XShm 共享内存；首版可不用，先保证正确性。
- 多显示器布局由 RandR 报告；负坐标和主屏偏移要保留。

#### 窗口枚举

- 读取根窗口 `_NET_CLIENT_LIST_STACKING`，从栈顶到栈底。
- 过滤 `_NET_WM_WINDOW_TYPE` 为 desktop/dock/tooltip 等。
- 用 `_NET_FRAME_EXTENTS` 补上窗口管理器边框。
- `TranslateCoordinates` 转换到根坐标；`GetWindowAttributes` 过滤未映射窗口。
- `_NET_WM_PID` 过滤 zchat 自身。
- 没有 EWMH 的轻量 WM：回退到 `XQueryTree` 顶层窗口列表。

### 6.4 Linux Wayland

Wayland 的安全模型不允许普通客户端读取整个桌面或任意定位覆盖窗口，因此分为“获取图像”和“编辑图像”两段。

#### 获取图像：Portal

- 通过 `github.com/godbus/dbus/v5` 连接 session bus。
- 调用 `org.freedesktop.portal.Desktop` 的 `org.freedesktop.portal.Screenshot.Screenshot`。
- 参数：
  - `parent_window`：可为空或 zchat 的 `xdg-foreign` handle（P1）。
  - `options`：`handle_token`、`interactive`、`modal`。
- 订阅 `org.freedesktop.portal.Request.Response` 信号，获取 `(response, results)`。
- `response == 0` 表示成功；`results["uri"]` 是 `file://` URI。
- 读取文件后删除，或移动到 zchat 缓存。
- 超时（如 30 秒）返回 `ErrCanceled` 或 `ErrCaptureFailed`。
- Portal 可能自己弹出区域选择或权限确认，这是正常的系统行为。

#### 获取图像：外部工具回退

按顺序探测：

1. `grim`（wlroots：Sway、Hyprland、Wayfire）
   - `grim -o <output> <png>`；输出名可通过 `wlr-randr` 获取。
2. `gnome-screenshot`
   - `gnome-screenshot -f <png>`。
3. `spectacle`（KDE）
   - `spectacle -b -n -o <png>`。
4. `xdg-desktop-portal` 已安装但未运行：提示用户检查会话。

所有命令使用 `context.WithTimeout`，退出码非零时返回可读错误。

#### 编辑图像

- Wayland 下不创建前置全屏 overlay。
- 获取 PNG 后，创建普通无边框窗口，显示图像并进入标注模式。
- 裁剪通过标注窗口内的裁剪框完成。
- 没有窗口自动识别，不做全局鼠标位置读取。
- 贴图窗口位置由合成器决定；不能保证绝对定位。

#### 全局快捷键

MyGo v0.7.4 的 `GlobalShortcut` 在 Wayland 下可通过 XDG portal 注册（GNOME 48+、KDE Plasma 6 等）。需要注意：

- 应用必须安装并有桌面入口，portal 才能识别；
- 桌面可能要求用户确认或改键；
- 旧桌面/未安装 portal 时注册失败；
- 失败后提供 `zchat --capture` 命令行入口，让用户绑定桌面快捷键。

### 6.5 其他平台

`capture_other.go` 返回 `ErrUnsupported`，UI 隐藏截图入口或显示“不支持当前平台”，不影响聊天功能。

## 7. 坐标与 DPI 模型

这是截图正确性的核心。

### 7.1 统一定义

- `Display.Bounds`：DIP，原点在主显示器左上角，允许负坐标。
- `Display.Scale`：物理像素 / DIP。
- `Frame.Image`：该显示器的物理像素图，左上角对应 `Display.Bounds.Min`。
- overlay 窗口的位置和大小为 DIP，和 `Display.Bounds` 一致。
- 标注操作坐标统一使用**原始截图的物理像素**，避免缩放误差累积。

### 7.2 转换公式

```go
func dipToPixel(d Display, p image.Point) image.Point {
    return image.Pt(
        int(math.Round(float64(p.X-d.Bounds.Min.X) * d.Scale)),
        int(math.Round(float64(p.Y-d.Bounds.Min.Y) * d.Scale)),
    )
}

func pixelToDip(d Display, p image.Point) image.Point {
    return image.Pt(
        d.Bounds.Min.X + int(math.Round(float64(p.X)/d.Scale)),
        d.Bounds.Min.Y + int(math.Round(float64(p.Y)/d.Scale)),
    )
}
```

选区裁剪：

```go
func cropRect(d Display, sel image.Rectangle, img image.Rectangle) image.Rectangle {
    min := dipToPixel(d, sel.Min)
    max := dipToPixel(d, sel.Max)
    r := image.Rect(min.X, min.Y, max.X, max.Y).Intersect(img.Bounds())
    if r.Dx() < 1 || r.Dy() < 1 {
        return image.Rectangle{}
    }
    return r
}
```

必须用 `max - min` 计算尺寸，而不是先算 DIP 宽度再乘缩放，否则小数缩放会出现 1 像素误差。

### 7.3 校准

- 启动截图时校验 `Frame.Image.Bounds().Dx()` 是否接近 `round(Bounds.Dx()*Scale)`。
- 如果不一致，以实际图像为准，更新 `Frame.Scale = imageWidth / Bounds.Dx()`，并记录一次日志。
- 混合 DPI、负坐标、主副屏交换、显示器热插拔都要有测试。
- 光标位置使用 `mygo.Screen.CursorScreenPoint()`（DIP），再通过目标显示器转换。

## 8. 截图会话生命周期

### 8.1 状态

```go
type shotSession struct {
    id       string
    display  capture.Display
    frame    *capture.Frame
    windows  []capture.WindowInfo
    bmp      *ui.Bitmap
    overlay  *mygo.Window
    mainShown bool
    phase    shotPhase
    sel      image.Rectangle
    tool     shotTool
    color    image.RGBA
    width    float32
    doc      annotate.Doc
    undo     []annotate.Op
    redo     []annotate.Op
    ...
}
```

`app` 只持有一个当前截图会话，防止重复触发。

### 8.2 启动流程

```text
startScreenshot(source)
  -> 若已有会话，取消旧会话
  -> 记录 mainWasVisible = a.win.IsVisible()
  -> 若设置要求，a.win.Hide()
  -> 等待 50~100ms（time.After，异步）
  -> 在 goroutine 中 capture.Displays / Capture
  -> post 回主线程
  -> 失败：恢复主窗口、提示
  -> 成功：创建 overlay、显示、Focus
```

要求：

- 不能在主线程阻塞等待截屏。
- 等待合成器的延时不能省；否则容易截到正在隐藏的主窗口。
- 截屏失败时，即使主窗口之前是隐藏到托盘的，也不要强行弹出。
- 记录触发来源（聊天页/菜单/快捷键），用于完成后决定是否恢复主窗口。

### 8.3 结束流程

```text
finish(action)
  -> 必要时生成最终 PNG
  -> 执行复制/保存/贴图/发送
  -> 关闭 overlay
  -> 恢复主窗口与焦点（如果之前可见）
  -> 清理临时文件、释放 bitmap
  -> 显示 toast / 切换到会话
```

失败时：

- 复制失败：保留 overlay，允许重试或保存。
- 保存失败：保留 overlay，弹出目录选择。
- 发送失败：保留截图，允许保存或重试。
- 贴图失败：提示错误，保留 overlay。

### 8.4 退出与异常

- `App.OnBeforeQuit` 关闭 overlay、贴图，删除未完成的临时文件。
- overlay 的 `OnClose` 阻止默认关闭并转为取消。
- 显示器变化时：如果 overlay 未打开，下一次截图重新枚举；如果已打开，取消本次截图。
- 应用进入后台/失去焦点时，不自动取消；用户仍可完成操作。

## 9. overlay 窗口设计

### 9.1 窗口参数

```go
mygo.NewWindow(mygo.WindowOptions{
    Title:            "zchat screenshot",
    Width:            d.Bounds.Dx(),
    Height:           d.Bounds.Dy(),
    Frameless:        true,
    AlwaysOnTop:      true,
    SkipTaskbar:      true,
    DisableShadow:    true,
    DisableResize:    true,
    DisableMove:      true,
    DisableMaximize:  true,
    DisableMinimize:  true,
    DisableClose:     true,
    BackgroundColor:  "#000000",
    Content:          ui.View(a.shotView),
})
```

- 创建后 `SetBounds(mygo.Rectangle{...})`，确保位置精确。
- macOS 优先 `Panel: true`，并 `SetVisibleOnAllWorkspaces(true)`；如果 Panel 在目标系统上不能覆盖全屏应用，回退普通无边框置顶窗口。
- Windows 使用 `AlwaysOnTop` + 全屏尺寸。macOS 不要用 `SetFullScreen`，避免进入独立 Space。Linux 必须使用全屏窗口：桌面顶栏/菜单栏位于普通置顶窗口之上的一层，普通窗口还会被限制在工作区内，顶部区域既无法显示也无法用鼠标选取。
- 不启用 `Transparent`，因为 overlay 始终绘制完整截图；透明会带来不必要的合成开销。
- `SetContentProtection` 默认关闭；它不能可靠阻止系统截图，且可能影响 overlay 自身显示。

### 9.2 视图结构

```text
Root Box (Fill, Focusable, Focus, HandleInput, Draw)
├── 截图与暗色遮罩（Draw）
├── 选区边框、手柄、尺寸标签（Draw）
├── 标注预览（Draw）
├── 放大镜（Draw）
├── 工具栏（绝对定位的 ui.Row / 按钮）
├── 颜色/线宽弹层（ui.Popover / Overlay）
└── 文字输入框（绝对定位的 ui.TextInput）
```

为什么用 `Draw` 而不是大量子元素：

- 截图和标注是像素级绘制，`ui.Painter` 更直接。
- 选区拖拽时避免每帧重建大量元素。
- 工具栏仍使用真实控件，保留可访问性和点击行为。

### 9.3 绘制顺序

1. 截图位图，铺满整个窗口。
2. 选区外暗色遮罩（四块矩形，避免整屏叠加）。
3. 标注操作（按时间顺序）。
4. 选区边框、辅助线、8 个手柄。
5. 尺寸标签、鼠标坐标、颜色信息。
6. 放大镜。
7. 工具栏（控件层）。
8. 临时拖拽中的标注（最后一个操作）。

### 9.4 性能

- `ui.NewBitmap(frame.Image)` 只创建一次；不要每帧从 `image.RGBA` 重建。
- `Draw` 中不进行缩放、模糊、PNG 编码等重操作。
- 马赛克在操作完成时预计算块颜色；拖拽预览只画色块。
- 文字预览使用 `Painter.Text`；不要每帧排版整段文字。
- 如果标注很多导致重绘慢，可把已完成的标注缓存到 `ui.Canvas`，但首版先保持简单。

### 9.5 焦点与键盘

- Root 设置 `Focusable().Focus()`。
- 文字输入框出现时，焦点交给输入框；提交/取消后还回 Root。
- 工具栏按钮不应抢走全局快捷键；Root 的 `HandleInput` 优先处理 Esc/Enter/Ctrl 组合。
- 使用 `c.OnShortcut` 时注意仅在 overlay 内有效；不要注册全局菜单快捷键来抢系统按键。
- 弹出颜色/线宽面板时，Esc 先关闭面板，再按一次取消截图。

## 10. 选区交互与状态机

### 10.1 状态

```text
Idle
  -> DraggingSelection
  -> Selected
  -> DraggingSelection / Resizing / MovingSelection
  -> DrawingAnnotation
  -> EditingText
  -> Finishing
  -> Closed
```

### 10.2 命中测试顺序

按下指针时，从上层到下层判断：

1. 工具栏/弹层（由真实控件处理，不进入画布命中）。
2. 文字输入框。
3. 选中标注的删除按钮/包围框。
4. 选区 8 个手柄（命中半径 8～10 DIP）。
5. 选区内部（移动）。
6. 已有标注（选择/移动）。
7. 画布空白（开始新选区或新标注）。

### 10.3 选区操作

- `Down`：记录起点，进入 `DraggingSelection`。
- `Move`：如果移动距离小于 3 DIP，仍视为单击候选；超过阈值后开始绘制矩形。
- `Up`：
  - 没有拖动：命中窗口则选中窗口；否则选择全屏。
  - 有拖动：规范化矩形，进入 `Selected`。
- 手柄拖拽：根据手柄方向调整 `sel.Min/Max`，确保最小 1×1 像素。
- 选区内部拖拽：整体平移，限制在显示器内。
- 键盘微调：方向键移动 1 像素（物理），`Shift` 调整大小。
- 吸附：在移动/调整时对边和角应用阈值；按住 `Alt` 跳过。

### 10.4 窗口高亮

- 打开 overlay 时预计算窗口列表和 DIP 边界。
- 鼠标移动时按 z-order 从上到下命中，忽略 overlay 自身。
- 高亮窗口边框 + 半透明填充。
- 单击选中窗口边界后，转换为选区。
- 窗口列表为空时，只支持拖动和全屏。

### 10.5 标注绘制

- 按下工具后，`Down` 开始临时操作，`Move` 更新，`Up` 提交到撤销栈。
- 矩形/椭圆/箭头/直线：记录起点和终点。
- 画笔/荧光笔：记录点序列；移动距离小于 1 像素时丢弃，避免点过多。
- 马赛克：记录矩形；完成时计算块颜色。
- 文字：点击位置创建输入框，提交时生成文字操作。
- 橡皮：点击/拖过标注时命中并删除；不修改底图。
- 所有操作使用物理像素坐标。

### 10.6 撤销/重做

```go
type History struct {
    ops  []annotate.Op
    redo []annotate.Op
}

func (h *History) Push(op annotate.Op)
func (h *History) Undo()
func (h *History) Redo()
func (h *History) Clear()
```

- 新操作入栈时清空 redo。
- 上限 100 步，超出丢弃最旧。
- 移动/缩放已有标注也作为一次操作入栈（可先做“删除 + 新增”的简化实现）。
- 撤销后重绘整个标注层。

## 11. 标注模型与渲染

### 11.1 数据模型

```go
package annotate

type Kind uint8

const (
    Rect Kind = iota
    Ellipse
    Arrow
    Line
    Pencil
    Highlighter
    Text
    Mosaic
    Blur
    Number
)

type Op struct {
    Kind     Kind
    Start    image.Point
    End      image.Point
    Points   []image.Point
    Rect     image.Rectangle
    Color    color.RGBA
    Width    float32
    Text     string
    FontSize float32
    Mosaic   int
}

type Doc struct {
    Width  int
    Height int
    Base   *image.RGBA
    Ops    []Op
}
```

### 11.2 裁剪

```go
func (d *Doc) Crop(r image.Rectangle) *image.RGBA
```

- 先把 `r` 与 `d.Base.Bounds()` 相交。
- 创建目标 `image.RGBA`，复制底图对应区域。
- 渲染操作时把坐标减去 `r.Min`。
- 输出尺寸至少 1×1。

### 11.3 预览

overlay 预览使用 `ui.Painter`：

- `Painter.Image` 绘制底图；
- `Painter.Fill`、`Line`、`Stroke`、`FillPath`、`StrokePath` 绘制几何工具；
- `Painter.Text` 绘制文字；
- 马赛克用色块 `Fill`；
- 所有坐标从 `image.Point` 转换为 overlay DIP：

```go
func (s *shotSession) dipRect(r image.Rectangle) ui.Rect {
    min := s.display.Bounds.Min.Add(pixelToDip(...))
    ...
}
```

预览不修改原始 `image.RGBA`。

### 11.4 导出

`internal/annotate/render.go` 使用纯 Go 光栅化：

- `golang.org/x/image/vector`：抗锯齿填充路径。
- `golang.org/x/image/font/opentype`：解析系统字体。
- `golang.org/x/image/font`：绘制字形。
- `image/draw`：底图复制、图像合成。
- 手写/复用粗线转多边形：画笔、箭头、矩形、椭圆的描边。
- 马赛克：对区域做块平均，再放大为方块。
- 模糊（P1）：可分离盒式模糊或 stack blur。
- 导出后使用 `image/png` 编码。

字体发现顺序：

| 平台 | 字体 |
| --- | --- |
| Windows | `msyh.ttc`（微软雅黑）、`segoeui.ttf` |
| macOS | `PingFang.ttc`、`SFNS.ttf` |
| Linux | Noto Sans CJK、DejaVu Sans；可用 `fc-match` 查找 |

字体加载失败时：

- 回退到内置的简单英文字体（如项目已有资源）；
- 文字仍可绘制，但中文可能显示为方框；
- UI 提示“当前系统缺少中文字体”，并允许改用形状标注。

### 11.5 预览与导出一致性

- 几何操作使用同一份 `Op` 数据，不分别维护两套状态。
- 预览使用系统矢量绘制，导出使用 CPU 光栅化；两者在抗锯齿和字体 hinting 上会有细微差异。
- 对矩形、椭圆、箭头、画笔、马赛克做黄金图像测试；文字只做非空/尺寸测试。
- 不把预览截图当作最终导出，避免精度损失。

### 11.6 PNG 与剪贴板

- 复制：`png.Encoder{CompressionLevel: png.BestSpeed}`，优先速度。
- 保存：`png.DefaultCompression`，优先体积。
- `mygo.Clipboard.WriteImage(pngBytes)`，返回错误时提示。
- Linux 下剪贴板由应用持有；应用退出后内容可能消失，这是平台行为，提示用户保存。

## 12. 放大镜与取色

- 从 `Frame.Image` 取以鼠标物理像素为中心的 `15×15` 或 `21×21` 区域。
- 在 overlay 上以 12× 放大绘制，每个源像素画成一个方块。
- 不创建新 `ui.Bitmap`，直接 `Painter.Fill` 绘制方块，避免频繁分配。
- 显示：
  - `X, Y`（物理像素）；
  - `#RRGGBB`；
  - 选区 `W × H`。
- 靠近屏幕边缘时，放大镜自动移到另一侧。
- 取色写入剪贴板文本；P1 可将颜色应用到标注。

## 13. 贴图窗口

### 13.1 数据结构

```go
type pinWindow struct {
    ID       string
    win      *mygo.Window
    bmp      *ui.Bitmap
    image    *image.RGBA
    opacity  float64
    ignore   bool
    created  time.Time
}
```

`app.pins map[string]*pinWindow`，所有操作在主线程。

### 13.2 窗口与视图

```go
w := mygo.NewWindow(mygo.WindowOptions{
    Title:        "zchat pin",
    Width:        imgW,
    Height:       imgH,
    Frameless:    true,
    Transparent:  true,
    AlwaysOnTop:  true,
    SkipTaskbar:  true,
    DisableShadow: true,
    Content:      ui.View(func(c *ui.Context) { pinView(c, p) }),
})
w.SetVisibleOnAllWorkspaces(true)
```

视图：

```go
ui.Box(c).Fill().Cursor(ui.CursorMove).DragWindow().Draw(func(p *ui.Painter, r ui.Rect) {
    p.Image(pin.bmp, r, ui.FillBox)
})
```

- `DragWindow` 让整个图片可拖动。
- 图片按窗口大小填充；窗口缩放时保持宽高比，通过 `SetBounds` 控制。
- 右键菜单使用 `ContextMenu` 或 `PopoverMenu`。
- 关闭时从 `app.pins` 移除并释放 bitmap。
- `SetIgnoreMouseEvents(true)` 实现鼠标穿透；托盘提供恢复入口。
- `SetOpacity` 调整透明度。
- 贴图窗口默认不持久化；退出时全部关闭。

### 13.3 缩放与命中

- 自定义四角/四边命中区（约 10 DIP），拖动时计算新窗口矩形。
- 保存原始宽高比，缩放时取宽高中变化较大的一边。
- `Ctrl/Cmd+滚轮` 调整窗口大小，步进 10%。
- 设置最小 40×40 DIP，最大不超过屏幕工作区。

### 13.4 平台注意

- macOS：`Panel` 可以让贴图浮在全屏应用上，但会失去部分普通窗口行为；按实机结果选择 Panel 或普通置顶。
- Windows：无边框窗口需要 `SetBounds` 控制位置；系统缩放边缘可能不可用，使用自定义手柄。
- Linux Wayland：合成器可能忽略 `SetPosition`，贴图位置无法绝对控制；文档标注限制。

## 14. 聊天集成

### 14.1 入口

- `ui_chat.go` 的 `composer` 增加截图图标按钮，调用 `a.startScreenshot(shotFromChat)`。
- `menu.go` 文件菜单增加“截图…”，快捷键显示设置中的值。
- `tray.go` 增加“截图”。

### 14.2 发送流程

```text
finishSend()
  -> 渲染最终 PNG
  -> 写入 cache/screenshots/shot_<id>.png
  -> 关闭 overlay
  -> a.showMain()
  -> 若无当前会话：提示/打开会话选择
  -> a.sendFiles([]string{path})
```

复用 `sendFiles` 的好处：

- 自动注册 TCP 令牌和 HTTP 回退；
- 自动识别图片 MIME、宽高；
- 自动创建图片消息；
- 复用发送进度、断点续传、缓存和旧版本兼容。

注意：

- `sendFiles` 只发送到 `a.currentConv()`；发送前必须保证会话存在。
- 不要删除已发送截图文件，消息记录中的 `File.Path` 仍会引用它。
- 未发送/取消的临时文件可以删除。
- 发送后保持主窗口可见，并停留在当前会话。

### 14.3 文件位置

| 用途 | 路径 |
| --- | --- |
| 临时截图 | `<cache>/screenshots/tmp-<id>.png` |
| 已发送截图 | `<cache>/screenshots/shot-<id>.png` |
| 历史 | `<cache>/screenshots/history/`（P1） |
| 用户保存 | `<设置的截图目录>/zchat-YYYYMMDD-HHMMSS.png` |

缓存目录使用现有 `a.store.CacheDir()`，避免新增路径体系。

## 15. 设置、快捷键与持久化

### 15.1 设置字段

在 `internal/app/settings.go` 增加：

```go
type settings struct {
    // ...existing...
    ScreenshotEnabled     bool   `json:"screenshotEnabled"`
    ScreenshotHotkey      string `json:"screenshotHotkey"`
    ScreenshotDir         string `json:"screenshotDir,omitempty"`
    ScreenshotAutoCopy    bool   `json:"screenshotAutoCopy"`
    ScreenshotAutoSave    bool   `json:"screenshotAutoSave"`
    ScreenshotCloseAfter  bool   `json:"screenshotCloseAfter"`
    ScreenshotCursor      bool   `json:"screenshotCursor"`
    ScreenshotDelay       int    `json:"screenshotDelay"`
    ScreenshotMagnifier   bool   `json:"screenshotMagnifier"`
    ScreenshotDim         int    `json:"screenshotDim"`
    ScreenshotSnap        bool   `json:"screenshotSnap"`
    ScreenshotPinTop      bool   `json:"screenshotPinTop"`
    ScreenshotHistory     bool   `json:"screenshotHistory"`
}
```

默认值：

- `ScreenshotEnabled = true`
- `ScreenshotHotkey = "F1"`
- `ScreenshotDir = <PathPictures>/xin.dart.zchat`
- `ScreenshotAutoCopy = false`
- `ScreenshotAutoSave = false`
- `ScreenshotCloseAfter = true`
- `ScreenshotCursor = false`
- `ScreenshotDelay = 0`
- `ScreenshotMagnifier = true`
- `ScreenshotDim = 45`
- `ScreenshotSnap = true`
- `ScreenshotPinTop = true`
- `ScreenshotHistory = false`（P1 默认开）

路径要复用 `normalizeDirForOS` 的思路，避免跨系统复制设置后创建非法路径。

### 15.2 快捷键注册

```go
func (a *app) applyScreenshotHotkey() {
    if a.shotHotkey != "" {
        mygo.GlobalShortcut.Unregister(a.shotHotkey)
        a.shotHotkey = ""
    }
    if !a.settings.ScreenshotEnabled {
        return
    }
    if err := mygo.GlobalShortcut.Register(a.settings.ScreenshotHotkey, func() {
        a.post(func() { a.startScreenshot(shotFromHotkey) })
    }); err != nil {
        a.shotHotkeyErr = err
        return
    }
    a.shotHotkey = a.settings.ScreenshotHotkey
}
```

- 不要调用 `UnregisterAll`，避免影响未来其他快捷键。
- 设置变更后重新注册。
- 注册失败在设置页显示，不弹阻塞对话框。
- `GlobalShortcut` 回调在主线程，但仍通过 `post` 统一状态修改。

### 15.3 第二实例

现有：

```go
mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
```

改为解析参数：

```go
mygo.App.OnSecondInstance(func(args []string, _ string) {
    if slices.Contains(args, "--capture") {
        a.post(func() { a.startScreenshot(shotFromCLI) })
        return
    }
    a.showMain()
})
```

这样 Wayland 用户可以把桌面快捷键绑定到 `zchat --capture`。

## 16. 线程与并发

- `app` 状态只在主线程读写，沿用现有 `post` / `mygo.RunOnMain` 模式。
- 平台截屏、窗口枚举、PNG 编码可以放到 goroutine；完成后通过 `post` 回主线程。
- overlay 的 UI 构建、绘制、输入处理全部在主线程。
- 贴图窗口创建/关闭在主线程。
- 全局快捷键回调通过 `post` 排队，避免重入。
- 用 `a.shot != nil` 防止重复触发；不要用锁保护 UI 状态。
- 显示器变化回调可能来自其他线程；只通过 `post` 设置标志。
- 截图完成前用户退出：设置 `a.quitting`，丢弃结果并清理临时文件。

## 17. 权限、隐私与安全

### 17.1 macOS

- 只申请屏幕录制权限。
- 首次触发时预检/请求；拒绝后给出系统设置路径。
- 不把权限请求放在应用启动时，避免首次打开就弹窗。
- 授权后旧系统需重启，提示用户。
- 权限被撤销后，下次截图重新提示。

### 17.2 Linux

- X11 没有系统级授权，任何本机应用都能读取屏幕；这是 X11 的固有限制。
- Wayland 通过 portal 授权；zchat 不绕过 portal。
- 外部工具回退需要用户已安装并信任这些工具。
- 不请求 root 权限。

### 17.3 Windows

- 普通桌面截图不需要额外权限。
- UAC 安全桌面、锁屏界面、受保护视频/DRM 内容可能无法捕获或为黑屏；不尝试绕过。
- 不安装驱动，不使用内核组件。

### 17.4 数据

- 临时文件权限：目录 `0700`，文件 `0600`。
- 日志不记录图像内容、窗口标题、剪贴板文本。
- 取消/失败后删除临时 PNG。
- 历史记录可清空；退出时可清理未发送临时文件。
- 不把截图写入 `state.json` 或 SQLite，除非作为聊天图片消息发送。

## 18. 性能目标与实现约束

| 场景 | 目标 | 措施 |
| --- | --- | --- |
| 单屏 4K 截图 | < 350ms 出现 overlay | 仅截当前显示器；GDI/CG 直接读像素；避免全屏多屏拼接 |
| 选区拖动 | 60fps | 位图只创建一次；Draw 中不做编码/模糊 |
| 标注拖拽 | 60fps | 操作数量少；马赛克预计算；不做全图重采样 |
| 复制 | < 150ms | `png.BestSpeed`；必要时异步 |
| 保存 | < 300ms | 默认压缩；大图可显示保存中 |
| 内存 | 单 4K 屏 < 250MB | 及时释放原始帧；限制历史；不缓存多份全图 |
| 多显示器 | 不一次性全截 | 默认当前屏；跨屏功能按需逐屏截取 |

其他约束：

- 不每帧调用 `image/png.Encode`。
- 不每帧创建 `ui.Bitmap`。
- 不在 `Draw` 中分配大对象。
- 大图编码可放到 goroutine，主线程显示进度/禁用按钮。

## 19. 测试策略

### 19.1 单元测试

- 显示器坐标转换：
  - 100%、125%、150%、200%、小数缩放；
  - 负坐标；
  - 主副屏交换；
  - 边界裁剪和 1×1 选区。
- 选区：
  - 反向拖动规范化；
  - 8 向手柄调整；
  - 移动限制；
  - 命中测试顺序；
  - 吸附阈值。
- 标注：
  - 所有操作的序列化/复制；
  - 撤销/重做；
  - 裁剪偏移；
  - 马赛克块计算；
  - 透明/半透明颜色；
  - 空操作和越界操作。
- 导出：
  - 小尺寸黄金图像（PNG）；
  - 文字非空/尺寸；
  - 1×1、全透明、极端宽高比。
- 文件：
  - 文件名生成、重名追加、非法字符；
  - 保存目录不存在/不可写；
  - 临时文件清理。
- 设置：
  - 默认值；
  - 旧设置反序列化；
  - 快捷键变更；
  - 跨系统路径修复。

### 19.2 平台像素转换测试

不依赖真实桌面，用合成缓冲区测试：

- Windows：32 位 BGRA、负高度、alpha；
- macOS：预乘 RGBA、sRGB、不同 bytesPerRow；
- X11：16/24/32 bpp、不同 mask、字节序。

这些函数必须是纯函数，便于 CI 在 Linux 上运行。

### 19.3 假后端集成测试

实现 `capture.Backend` 的 fake：

- 固定显示器、固定图像、固定窗口列表；
- 驱动完整会话：启动、选区、标注、复制、保存、取消；
- 注入错误：权限拒绝、无显示器、截图失败、保存失败；
- 验证主窗口恢复、临时文件清理、状态机不泄漏。

### 19.4 手工验收矩阵

| 维度 | 取值 |
| --- | --- |
| Windows | 10、11；100/125/150/200%；单屏/双屏；负坐标 |
| macOS | 12/13/14/15（可测版本）；Intel/Apple Silicon；Retina/普通屏 |
| Linux X11 | GNOME/KDE/XFCE 至少两种；100/200%；多屏 |
| Linux Wayland | GNOME 48+/KDE 6；portal 成功/取消/拒绝；无 portal |
| 场景 | 全屏、窗口、区域、极小选区、贴图、发送、保存、取消 |
| 异常 | 断电/退出/关闭主窗/权限撤销/显示器热插拔 |

### 19.5 CI

- `go test ./internal/...`
- `go test -race ./internal/...`
- `go vet ./...`
- `go tool mygo vet`
- `go tool mygo build -platform windows/amd64`、`darwin/universal`、`linux/amd64`
- 平台后端通过 build tag 编译，确保没有 cgo。

## 20. 依赖与构建

### 20.1 新增依赖建议

| 依赖 | 用途 | 说明 |
| --- | --- | --- |
| `github.com/ebitengine/purego` | macOS/可选 Xlib 动态调用 | 已是 MyGo 间接依赖，提升为直接依赖 |
| `golang.org/x/sys` | Windows/macOS/Linux 系统调用 | 已是间接依赖 |
| `golang.org/x/image` | 光栅化、字体、PNG 辅助 | 已是直接依赖 |
| `github.com/jezek/xgb` | Linux X11 协议 | 纯 Go，避免 cgo；如不采用则用 purego libX11 |
| `github.com/godbus/dbus/v5` | Wayland portal | 纯 Go，无 cgo |

约束：

- 所有依赖必须支持 Windows/macOS/Linux 交叉编译。
- 不引入需要 CGO 的图形库。
- 不为首版引入大型图像处理库；标注渲染优先使用 `x/image` 自研小型后端。

### 20.2 build tag

```text
capture_windows.go   //go:build windows
capture_darwin.go    //go:build darwin
capture_linux.go     //go:build linux
capture_other.go     //go:build !windows && !darwin && !linux
```

Linux 内部再按运行时检测 X11/Wayland，不用构建 tag 区分，因为同一个二进制可能运行在两种会话中。

### 20.3 对现有构建的影响

- 不改变 `mygo build` 的调用方式。
- 不引入 cgo，Linux 仍不需要 gcc 之外的额外运行时。
- macOS 仍用系统 CoreGraphics；不需要额外框架安装。
- Windows 仍使用系统 DLL。
- Wayland 用户需要 `xdg-desktop-portal` 或外部工具，属于运行时依赖，不是构建依赖。

## 21. 实施顺序

1. **M0-1 类型与假后端**：`capture` 类型、坐标转换、fake backend、单元测试。
2. **M0-2 Windows 截屏 POC**：截当前显示器、窗口枚举、DPI 验证。
3. **M1 overlay + 选区 + 复制**：最小可用截图闭环。
4. **M2 标注模型 + 预览 + 导出**：先矩形/箭头/画笔，再文字/马赛克。
5. **M3 聊天集成**：按钮、菜单、发送、设置、快捷键。
6. **M4 贴图**：窗口、拖动、缩放、透明度、关闭。
7. **M5 macOS**：权限、CoreGraphics、Retina、多屏。
8. **M6 Linux X11**：xgb、窗口枚举、多屏。
9. **M7 Wayland**：portal、外部工具、编辑器模式。
10. **M8 加固**：多显示器、性能、错误处理、i18n、构建、手工矩阵。

每一步都应保持可编译、可测试；平台后端未完成时用 `ErrUnsupported` 占位。

## 22. 设计决策记录

| 决策 | 选择 | 原因 |
| --- | --- | --- |
| 截图 API | 自研平台后端 | MyGo 没有桌面截屏 API |
| overlay 窗口 | 每显示器一个，首版只当前屏 | 混合 DPI 下比单窗口跨屏更可靠 |
| 标注坐标 | 原始物理像素 | 避免缩放误差，导出精确 |
| 预览/导出 | 同操作模型，双后端 | 预览要快，导出要精确 |
| 发送图片 | 复用 `sendFiles` | 不新增协议，自动获得传输/进度/兼容 |
| 贴图窗口 | 每贴图一个窗口 | 简单、独立、可多贴图 |
| Wayland | 门户/工具先截图，zchat 后编辑 | 平台安全模型限制，无法前置覆盖 |
| 快捷键 | MyGo GlobalShortcut + CLI 回退 | Wayland 兼容性 |
| 历史 | 独立文件，不进 SQLite | 截图不是聊天记录，生命周期不同 |

## 23. 已知限制

1. Wayland 下无法在截图前显示 zchat 的全屏 overlay，也无法做窗口自动识别。
2. Wayland 下贴图位置由合成器决定，不能保证绝对定位。
3. Windows HDR/受保护内容可能颜色异常或黑屏。
4. macOS 新版系统对旧 CoreGraphics 截屏 API 的长期支持不确定。
5. 文字标注的预览与导出可能存在轻微字体渲染差异。
6. 跨显示器连续选区不在首版范围。
7. 截图内容会被其他屏幕录制/截图软件捕获，平台通常无法完全阻止。
8. X11 没有系统级屏幕读取授权，任何本机应用都可以截屏。

## 24. 待确认技术问题

1. Linux X11 采用 `jezek/xgb` 还是 purego `libX11`？前者更纯，后者与 MyGo 的动态加载方式一致。
2. macOS 是否需要在首版就引入 ScreenCaptureKit，还是先 CoreGraphics + `screencapture` 回退？
3. `F1` 在目标用户的系统上冲突率是否可接受？
4. 文字导出使用系统字体还是内置字体？内置 CJK 字体会显著增大安装包。
5. 是否需要把标注操作持久化，以支持贴图重新编辑？首版建议不持久化。
6. 截图历史是否进入 P0？如果进入，需要额外的清理和隐私说明。
7. Wayland portal 的 `interactive` 参数在不同桌面上的行为差异，需要实机验证。
8. 贴图是否需要在主窗口关闭后继续存活，以及如何避免应用误退出。
