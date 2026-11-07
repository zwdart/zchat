# 图标源文件

`resources/` 下的 SVG 是源文件，请保留，不要删除；`build/` 中的副本只是构建产物。

| 源文件 | 生成 |
| --- | --- |
| `resources/icon.svg` | `resources/icon.png` |
| `resources/icon-dev.svg` | `resources/icon-dev.png` |

重新生成：

```bash
rsvg-convert -w 1024 -h 1024 resources/icon.svg -o resources/icon.png
rsvg-convert -w 1024 -h 1024 resources/icon-dev.svg -o resources/icon-dev.png
```

- 正式版使用蓝色渐变圆角方块、白色对话气泡和圆角 Z。
- 开发版使用橙色渐变，并在右上角显示 `DEV` 标识。
- 托盘直接复用应用图标：正式版使用 `icon.png`，`mygo dev` 使用 `icon-dev.png`；不维护单独的托盘图标。
