# Planet Windows 开发交接

更新于 2026-11-04，基线版本为 `0.3.0`。2026-11-06 补充：首个公开版本目标已改为 `0.0.1-alpha.1`；五个模板继续内置，本项目移除模板待审发布门槛并保留来源与署名；Kubo 的四项许可材料缺项已补齐。下文构建及验收数字仍记录原基线，最新范围见[发行检查记录](release-audit.md)。

## 工作区

- 后续开发目录为 `D:\iip\planet-windows-open-source`。基线提交 `6c5cca5` 已推送到 `origin/main`，远程仓库为 `Stdq123/planet-windows`。[GitHub 提交](https://github.com/Stdq123/planet-windows/commit/6c5cca5e028cfde8037e574ceaa9eccf5c61cc19)
- `D:\iip\Planet` 是 macOS 原版参考，`D:\iip\planet-windows` 是旧开发工作区。后续改动和提交都在 `planet-windows-open-source` 进行。
- 项目是 macOS Planet 的独立 Windows 移植，目标是尽量覆盖原版功能。技术路线为 Go、MyGo、WebView2 和 TypeScript，明确排除 Electron 和 Swift。默认使用 PowerShell 和 UTF-8。

## 已实现

| 模块 | 当前能力 |
| --- | --- |
| 创作与发布 | 多站点、Markdown 文章、阅读及编辑、私有草稿、附件、预览、静态 HTML 导出。本地站点支持文章自定义路径、文章/页面、外链跳转及按权重排序的站点导航；支持头像、站点标识、作者和搜索引擎收录设置。保存文章及文章设置后，后台重新发布整个站点。Kubo 添加站点文件并更新 IPNS。 |
| 导入、导出与模板 | 支持部分原版 `.planet` 备份的导入和导出。兼容的导入站点可编辑文章和附件，并使用原 IPNS 身份重新发布。内置已验证的 Plain、8-bit、Grid、Platinum、Sepia 指定版本，提供示例预览及手动目录导入；Sepia 支持部分原版设置。Paper 目前只验证了归档渲染。 |
| 订阅 | 支持 RSS、Atom、JSON Feed、Planet/IPNS 和受支持的 ENS 站点，以及自动刷新、收藏、已读/未读、按订阅全部标为已读、今天和未读列表、离线固定和媒体播放。可搜索本地文章及缓存的订阅内容。 |
| 集成与管理 | 支持 Filebase 自动固定及状态查询、Pinnable 固定、Plausible 站点专属脚本及统计查询，以及站点移除、恢复和永久删除。本地站点支持自定义域名、社交资料、播客和 SSH/Cloudflare Pages 发布。 |
| 安装与更新 | 安装器支持从兼容的 0.2 版事务升级到 0.3.0、中断后重试和资料库迁移。应用支持配置可信 HTTPS 源、验证 Ed25519 签名及安装包摘要。下载后由用户手动运行安装器。 |

## 已知问题与验证边界

- **公网访问**。此前实测，测试站点的 IPNS 签名记录可解析到正确 CID；补充固定和公告后，独立节点及 Aleph Cloud 网关曾成功获取首页和文章，`eth.sucks` 当时仍超时。公网可用性会变化，原始失败的根因尚未确认。侧栏现可选择 `ipfs.io`、`eth.sucks`、`dweb.link` 和 Aleph Cloud，复制地址及 Planet 文章分享链接使用所选网关。[公开网关指南](eth-sucks-guide.md)
- **节点状态与界面**。侧栏提供 Kubo 状态、运行开关、本地网关、仓库大小、手动清理未固定数据和流量历史图；面板打开期间每 5 秒采样，历史最多 5 分钟，不跨进程持久化。开关关闭状态持久化，网络操作及 IPNS 续期不会自动重启节点；重新开启后前端重试待发布站点。节点仅由本程序管理。常用操作位于顶栏，其他站点操作放在“更多操作”，备份和公共网关位于侧栏“更多工具”。已用隔离示例数据检查约 1268×814 和 914×814 窗口的主界面、编辑和节点面板；本轮在约 1268×814 窗口目视检查了文章设置和站点设置，并调整文章类型下拉框。整体视觉与原版仍有差距。
- **新站点设置**。域名和社交资料进入公开元数据及页面，ENS 复制地址使用所选网关；解析记录及 HTTPS 服务仍需用户自行配置。含音频的文章生成 `podcast.xml`，封面、语言和分类可配置。导入站点不能修改新设置，新增公开设置的导入及真实 Mac 双向兼容仍待完成。
- **外部发布**。Cloudflare 使用 Direct Upload（每文件 25 MiB、总计 256 MiB），令牌由当前 Windows 用户的 DPAPI 保护且不进入备份；SSH 需要兼容 Cygwin 路径的 rsync、OpenSSH、已信任的主机密钥及非交互式认证，本机尚未安装 rsync。手动发布独立于 IPFS，自动同步仅在 IPNS 成功发布后执行；远端删除默认关闭。远端结果不确定时须先检查目的地再手动重试。两者仅通过模拟 API 和假 rsync 进程测试，尚未用真实账号或服务器验证。
- **Clash Verge**。此前用户授权为历史构建的 `ipfs.exe` 完整路径设置 DIRECT 规则，`eth.sucks` 域名也有 DIRECT 规则。每次构建的目录 ID 不同，旧进程路径规则不会自动匹配新构建；当前最新构建路径尚未加入规则。用户暂时不能关闭 TUN。
- **备份与模板**。构造样本已覆盖导入、编辑、重建、导出和重导入，仍缺真实 macOS `.planet` 备份的双向验证。部分特殊模板设置或文件会触发保守拒绝。测试使用临时资料库和测试密钥，避免改动或重新公开现有测试站点。
- **内置模板来源**。Plain、8-bit、Grid、Platinum、Sepia 均保留固定版本上游链接、素材署名及已有许可文本；Sepia 上游提供 MIT 许可证。发行包第三方声明保留这些材料，不另行为模板或素材指定许可证。
- **安装与更新**。真实 0.2.0 和 0.3.0 安装包已通过隔离 Wine 升级和卸载测试。原生 Windows 已验证独立启动和资料库迁移，完整原生安装生命周期及断电恢复待验证。生产更新源、Authenticode 签名和依赖许可复核仍待完成。[0.3.0 验收记录](acceptance-0.3.0.md)
- **外部服务**。Filebase、Plausible 和 Pinnable 已使用模拟服务测试，真实账号联调待验证。

## 代码入口

| 范围 | 位置 |
| --- | --- |
| 应用入口 | [Go 主程序](../main.go)、[TypeScript 界面](../src/main.ts) |
| 发布与 Kubo | [publisher](../internal/publisher)、[ipfs](../internal/ipfs) |
| 备份、迁移与页面生成 | [backup](../internal/backup)、[migration](../internal/migration)、[sitegen](../internal/sitegen) |
| 签名更新与安装 | [updates](../internal/updates)、[清单签名工具](../cmd/update-manifest)、[升级与恢复说明](safe-upgrade.md) |

## 本地检查与构建

```powershell
Set-Location 'D:\iip\planet-windows-open-source\windows'
bun install --frozen-lockfile
bun test
bun run typecheck
go test ./...
```

本轮 89 项前端测试、类型检查和 `go test ./...` 通过，五个内置模板均验证了解包、校验、示例渲染和安装；隔离资料库的实际窗口检查显示 Platinum 预览及模板列表。此前新增设置弹窗尚未完成实际窗口目视验收。最新构建已同步到 `build/windows-amd64/`，独立发行目录为 `.mygo/releases/f0f2b1f6f0f24f2cb7e45778bb5ecb79/windows-amd64/`。Kubo 依赖仍有 4 个模块待人工许可复核，程序及安装包未签名。

需要打包时，在同一目录运行 `bun run build`。检查通过后，最新版会同步到 `build/windows-amd64/`，独立构建仍保存在 `.mygo/releases/<构建ID>/windows-amd64/`；旧的固定目录会保存为 `build/.previous-<ID>/`。从固定目录运行的 Planet 和 Kubo 需关闭后才能替换该目录。环境要求和可选集成测试参数见 [Windows 开发文档](../README.md)。
