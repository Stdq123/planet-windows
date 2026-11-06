# 设计规范修复：第一批无障碍 P0-A

日期：2026-11-06。基线：`main` 的 `8831b0a`。工作分支：`fix/design-spec-a11y`。

本批依据提供的 Interface Atlas 设计规范与合规报告修复 A1–A5，不重新审计，不包含第二批视觉 P0-B。保留现有依赖、组件体系和样式加载顺序；不改 Go、发布与更新业务、发行元数据或发布材料。本记录承载本批变更说明。

## 修复与测试对应

| 项目 | 实现 | 回归覆盖 |
| --- | --- | --- |
| A1 编辑器焦点 | [refinement.css](../src/refinement.css) 恢复标题与正文的 `2px solid var(--focus)`、`outline-offset: 2px`。只放开正文两个外层容器的裁切，textarea 保持自身滚动。 | [focus-styles.test.ts](../tests/focus-styles.test.ts) 按实际加载顺序检查完整 CSS 层叠。 |
| A2 跳到主要内容 | [index.html](../index.html) 首位跳转链接，默认视觉隐藏、聚焦可见。仪表盘指向 `#dashboard`，其他模式指向 `#workspace-main`；两处 main 均有 `tabindex="-1"`。[main.ts](../src/main.ts) 同步目标并移动焦点。主区域贴窗口边缘，使用向内 4px 的 outline offset，避免外框被视口裁切。 | [dashboard-navigation.test.ts](../tests/dashboard-navigation.test.ts) 覆盖仪表盘、编辑、收藏及返回；样式测试覆盖链接显隐与主区域焦点。 |
| A3 对话框关闭回焦 | [ui.ts](../src/ui.ts) 集中记录打开来源，关闭时依次尝试有效触发器、所属菜单 summary、当前可见主内容；保护其他打开的模态。模板设置和固定服务异步入口显式传入来源。删除原发布信息弹层的重复回焦特判，保留 `mayDismiss` 未保存与忙碌保护。 | [dialog-focus.test.ts](../tests/dialog-focus.test.ts) 覆盖菜单折叠、不可用入口、重开、嵌套、取消、保存中保护，以及两个真实异步入口；保留 [workspace-ui.test.ts](../tests/workspace-ui.test.ts) 既有断言。 |
| A4 字段错误关联 | [form-errors.ts](../src/form-errors.ts) 统一原生约束、自定义校验及已知服务端字段错误，设置 `aria-invalid`、追加 `aria-describedby`，按表单顺序聚焦首错。接入站点设置、文章设置和 [X 同步](../src/x-sync.ts)。改正、重试、重开及成功停用清理旧错误，保留原有说明和输入。通用网络或存储失败保留状态提示，不猜测错误字段。 | [form-errors.test.ts](../tests/form-errors.test.ts)、[site-settings.test.ts](../tests/site-settings.test.ts)、[article-settings.test.ts](../tests/article-settings.test.ts)、[x-sync.test.ts](../tests/x-sync.test.ts)。 |
| A5 模板选择语义 | [main.ts](../src/main.ts) 使用按钮 `aria-pressed`，同步 [layout.css](../src/layout.css) 选择器。保留按钮方案，因为现有操作是选择预览并单独应用，原生 Tab、Enter、Space 路径无需引入 listbox 的方向键与焦点模型。 | [builtin-templates.test.ts](../tests/builtin-templates.test.ts) 覆盖初始项、切换及再次激活同一项，确认没有残留 `aria-selected`。 |

## 验证边界

- 基线完整 `bun run check` 已通过：250 个前端测试，无失败；Go 格式、静态分析、无缓存测试、TypeScript 和生产构建通过。
- 修复先用回归测试复现，再通过相关测试。最终验收命令仍是在 `windows/` 执行 `bun run check`，结果见本次交付输出。
- 使用内置浏览器打开隔离的本地测试页面，加载工作区原有样式顺序，在 1280×720 视口检查标题、正文、跳转链接及两个 main 的焦点。截图与边界测量均显示焦点指示可见、无祖先裁切。这只是浏览器渲染检查，不代表原生桌面端验收。
- WebView2 原生焦点、Tab/Shift+Tab/Escape 完整操作路径：**未实机验证**。
- Narrator 对错误关联、模板按下状态及回焦的实际朗读：**未实机验证**。
- 200% 缩放下的焦点、裁切和布局：**未实机验证**。
- 强制颜色模式、可选 Kubo/在线/外部模板集成、安装器验收未纳入本批执行。

第一批交付后停止，等待维护者确认后再开展视觉 P0-B。
