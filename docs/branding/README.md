# Planet Windows 标志

Planet Windows 使用 **Neighbor Pages（邻页）** 标志。两枚相对的页片围出开放留白，表达“各自独立，彼此相连”的写作、阅读与发布关系。这是选定的 C 方案，用于 Windows 移植版的产品标识。

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="planet-horizontal-reversed.svg">
  <img src="planet-horizontal-color.svg" alt="Planet Windows 邻页标志" width="300">
</picture>

## 配色与使用

| 颜色 | HEX | 用途 |
| --- | --- | --- |
| 炭黑 | `#333432` | 默认单色标志、字标、主应用图标底色 |
| 暖白 | `#FBFAF7` | 浅色画布、深底反白标志 |
| 墨绿 | `#4F6B5B` | 浅底双色标志的下方页片、辅助色 |

双色符号用于至少 48 px 的浅色场景；小尺寸使用单色轮廓。炭黑与暖白的对比度约为 11.99:1，墨绿与暖白约为 5.61:1。深色背景应使用反白版本，不把炭黑字标或墨绿符号直接放在炭黑底上。

保持长宽比及两个页片的相对位置，标志四周至少留出一个竖笔画宽度的空白。不要添加描边、阴影、渐变或旋转页片。优先使用 SVG 母版导出所需尺寸，避免放大小 PNG。

## 应用内资源入口

以下路径均相对于仓库根目录：

| 文件 | 用途 |
| --- | --- |
| `windows/resources/icon.png` | 1024×1024 主应用图标；MyGo 读取该 PNG 并在构建时生成桌面图标资源 |
| `windows/resources/icon.ico` | 安装器与卸载器的专用图标，通过 NSIS MUI 配置嵌入；包含 16-256 px 的 15 档尺寸 |
| `windows/resources/workspace/planet.svg` | 侧栏 27 px 品牌符号；采用专用小尺寸单色轮廓 |
| `windows/resources/favicon.svg` | 网页预览的 SVG favicon，按浅色或深色偏好切换符号颜色 |
| `windows/resources/favicon.ico` | 网页预览的 ICO 后备图标，采用炭黑底反白符号 |

这些文件仅标识 Planet Windows 应用和开发预览，不替换用户自有站点的头像、订阅图标或导出站点内容。桌面图标变更需重新构建应用；不能仅复制 ICO 来更新已有 EXE 中的图标。

## 保留的维护母版

- [双色水平组合](planet-horizontal-color.svg)：README 和浅底宣传材料。
- [单色水平组合](planet-horizontal-mono.svg)：单色排版。
- [反白水平组合](planet-horizontal-reversed.svg)：深色背景及 README 深色主题。
- [双色堆叠组合](planet-stacked-color.svg)：窄版或方形布局。
- [独立单色字标](planet-wordmark-mono.svg)：已有符号的排版场景。

全部标志与字标由原创矢量路径构成，无需安装字体，也不包含嵌入位图。不要重新键入“Planet”来替代字标。界面正文仍遵循项目自身的字体与配色规范。

本目录保留日常维护所需的少量 SVG，不重复收录完整交付包。印刷色需按纸张与工艺另行打样；文件的渲染与结构检查不代表商标检索或实际 Windows 桌面验收。
