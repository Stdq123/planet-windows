# Planet Windows UI/UX 设计参考

状态：参考基线，尚未按本规范完成全界面设计或验收。整理日期：2026-11-04。

## 基础来源

- **全局设计系统：[Microsoft Windows UI Kit 与 WinUI 3 Gallery](https://learn.microsoft.com/en-us/windows/apps/design/downloads/)**。Microsoft 的 Windows UI Kit for Figma 提供按钮、导航、对话框等组件，及字体、颜色、间距等样式。该页面同时提供 UI Kit 和 Gallery 的入口。
- **阅读器页面结构：[RSS Reader Column View — Kristiana](https://dribbble.com/shots/12481440-RSS-Reader-Column-View)**。参考左侧订阅导航、中间文章列表、右侧阅读区的布局；该图是设计案例，不是覆盖所有状态的设计系统。
- **编辑器视觉参考：[Text Editor - Concept — Roshan](https://dribbble.com/shots/15519039-Text-Editor-Concept)**。参考正文与导航的层级；该作品是 Markdown 桌面编辑器概念图，不包含 Planet 的完整发布和草稿流程。

## 如何应用到 Planet

| 范围 | 采用的现成参考 | 应用边界 |
| --- | --- | --- |
| 导航、文章列表、阅读区 | RSS Reader Column View；[Windows 布局和间距](https://learn.microsoft.com/windows/uwp/design/layout/alignment-margin-padding) | 以现有三栏的业务结构为基础；窗口缩小时需验证可访问的收纳和恢复方式。案例图不是响应式行为规范。 |
| 字体 | [Windows Typography](https://learn.microsoft.com/en-us/windows/apps/design/signature-experiences/typography) | 参考 Segoe UI Variable；中文使用合适的系统回退字体。以 Windows 的正文 14/20、辅助文字 12/16、次级标题 20/28、标题 28/36 为基准。原文单位为有效像素，WebView 中需结合缩放实际验证。 |
| 颜色和主题 | [Fluent 2 Design Tokens](https://fluent2.microsoft.design/design-tokens)、[Windows 对比度主题](https://learn.microsoft.com/en-us/windows/apps/design/accessibility/high-contrast-themes) | 按前景、背景、边框、强调和状态的语义选择官方设计令牌；分别验证浅色、深色和对比度主题。不要把不同主题的颜色混用。 |
| 间距与对齐 | [Windows 布局和间距](https://learn.microsoft.com/windows/uwp/design/layout/alignment-margin-padding) | 固定尺寸、间距优先参考 4 有效像素的倍数；保持同类容器的边距与对齐一致。原文对宽窗口建议 24 有效像素的边距，具体应用到各栏须结合实际空间验证。 |
| 圆角 | [Windows 11 Geometry](https://learn.microsoft.com/en-us/windows/apps/design/signature-experiences/geometry) | 页面内按钮和列表底板参考 4px；弹出菜单和对话框参考 8px；相接边缘遵循官方例外规则。 |
| 图标 | [Segoe Fluent Icons](https://learn.microsoft.com/en-us/windows/apps/design/iconography/segoe-fluent-icons-font) | 采用同一套官方图标，优先 16、20、24 等官方推荐尺寸；图标需有可访问名称。Windows 10 不默认包含该字体，必须验证回退或使用合适的可分发资源。 |
| 按钮及更多操作 | [Windows Buttons](https://learn.microsoft.com/en-us/windows/apps/design/controls/buttons)、[Menus and context menus](https://learn.microsoft.com/en-us/windows/apps/develop/ui/controls/menus-and-context-menus) | “更多操作”对应 DropDownButton 和 MenuFlyout 的样式与交互；常用操作优先直接呈现，菜单中的命令按用途组织。应同时检查默认、悬停、按下、禁用与键盘焦点状态。 |
| 可访问性 | [Accessible text requirements](https://learn.microsoft.com/en-us/windows/apps/design/accessibility/accessible-text-requirements)、[Accessibility testing](https://learn.microsoft.com/en-us/windows/apps/design/accessibility/accessibility-testing) | 可见文本的前景/背景对比度以至少 4.5:1 为基线，依原文处理例外；检查键盘、Narrator、系统缩放和对比度主题，不只依赖颜色表达状态。 |

## 落地范围与限制

Planet Windows 使用 WebView 和 TypeScript，不是直接使用 XAML 控件的 WinUI 应用。上述材料是设计和交互参考；Windows UI Kit 的组件外观、设计令牌和系统行为需要映射到现有技术栈，不能声称自动继承 WinUI 的无障碍或主题能力。

实施时应覆盖：首次启动与空状态、站点和订阅导航、文章列表、阅读器、编辑和预览、菜单、设置、IPFS 状态、错误及加载状态。对宽窗口和接近最小尺寸的窗口分别检查，另外覆盖系统缩放与键盘操作。

Dribbble 作品用于设计参考；复用原图、品牌、字体或设计资源前应查看作者及平台的许可。Windows UI Kit 的使用许可也应以其资源页面为准。
