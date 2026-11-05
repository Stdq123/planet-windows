# 工程建设验证记录（2026-11-05）

本记录覆盖本地质量检查、发布工具与 GitHub 规则建设，不构成正式发行批准。工作分支为 `chore/git-workflow-foundation`；本轮未推送代码、创建公开 Release 或运行云端 CI。

## 已执行

- `bun run check`：122 个前端及发布工具测试通过，Go 格式、`go vet`、禁用结果缓存的 Go 测试、TypeScript 和前端生产构建通过。
- 发布工具包含 7 个元数据用例、6 个 PowerShell 编排用例，覆盖草稿、正式模式、脏工作区、轻量标签、标签提交不匹配、未签名、许可待审与复制摘要不符。编排用例使用合成审计结果，不代表真实证书验收。
- Git 测试夹具在继承强制提交/标签签名及不可用签名器的环境下仍通过，夹具不会调用开发者签名设备。
- Markdown 脚本 URL 回归在旧 goldmark 上三个案例均失败，升级后通过；预览、订阅解析和站点生成回归通过。
- `bun run security:check`：Bun 无已知漏洞报告；Go 扫描可达漏洞从 3 个降为 0。已更新 `golang.org/x/net` 至 0.62.0、`goldmark` 至 1.7.17，以及所需关联依赖。
- Go 详细扫描仍列出未发现可达调用的 [GO-2026-5158](https://pkg.go.dev/vuln/GO-2026-5158)（OpenTelemetry baggage）和 [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)（未使用的 OpenPGP 包）。不将可达扫描结果表述为所有依赖完全无漏洞；后续使用路径变化需重新扫描。该检查不覆盖独立 Kubo 二进制及模板资产。
- `bun run build`：完成 Windows 程序及独立安装包构建，程序 `--planet-version` 检查返回 0.3.0，10 项安装 payload 与独立文件逐项一致。
- `test-release-audit.ps1` 验证正确 payload 被接受、篡改 payload 被拒绝；`test-publish-build.ps1` 验证旧构建保留及失败时不覆盖现有产物。
- 实际安装包通过 `release:prepare` 生成标为 DRAFT 的本地材料，核对清单、安装包副本和 SHA-256 一致。
- UTF-8、LF、Markdown 相对链接和 GitHub YAML/JSON 校验通过；工作流仅手动触发，Actions 固定到完整 SHA。

## 本地构建证据

- 独立发行目录：`windows/.mygo/releases/414b2e201fbb448aaa2a442ffe70505a/windows-amd64/`。
- 安装包：`planet-windows-safe-setup-0.3.0-amd64.exe`，58,439,211 字节。
- SHA-256：`8cf65398173a051002d27e74e6c38fca5c694ef144964b77dc211bde8ccd7375`。
- 草稿材料：`windows/.mygo/release-drafts/0.3.0-f8135a65cd404593aae4cab6666af5fd/`。
- 程序及安装器 Authenticode：`NotSigned`；Kubo 许可待审计数：4。
- 仍沿用开发配置版本 0.3.0，未创建发行标签或补造历史发行记录；当前快照会被正式元数据检查拒绝。

## 服务端核验

主分支规则 `main-protection`（26594760）与标签规则 `release-tag-protection`（26594762）均为 active；管理员保留应急绕过。私密漏洞报告、密钥扫描、推送保护均开启。Windows checks 工作流保持 `disabled_manually`。未启用必需 CI 检查或 CodeQL 自动工作流；发布分类所需标签已配置。

## 正式发行前仍需完成

受信任发布者签名、完整第三方许可审查、隔离原生 Windows 的安装/升级/卸载、声明支持的真实 Mac 备份往返和最终发布者身份验证。依照 [验收模板](release-acceptance-template.md) 保存实际证据，不能以本记录代替。安装包和草稿材料保存在忽略目录，不进入源码仓库。
