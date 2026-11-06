# Telegram Sync Implementation Plan

**Goal:** 每个站点支持一个 Telegram 群组或频道，文章发布后同步标题、摘要与链接。

**Architecture:** 私有 Telegram 服务负责 API、加密配置、去重和发送状态；应用层连接已发布快照；现有站点菜单提供配置和重试。

**Tech Stack:** Go、MyGo、TypeScript、Bun，复用现有依赖。

**Spec:** ../specs/2026-11-06-telegram-sync-design.md

## Constraints and review focus

- PowerShell / UTF-8；仅修改 Windows 项目。
- Token 不进入前端查询结果、公开网站、备份或错误消息。
- 发布失败、草稿、未发布修改不能触发 Telegram。
- 网络结果不确定、站点切换和目标变更不能产生隐式重复或错发。
- 保持 IPNS 发布结果与 Telegram 警告分开。

## Tasks

- [x] 服务：新增 internal/services/telegram.go 与测试。实现 TelegramAPI、TelegramConfig、TelegramPost、Load/Save/DeleteTelegramConfig、SyncTelegram；使用 mock HTTP 验证成功、拒绝、重定向、超时、去重与重试。
- [x] 应用：新增 telegram.go 与测试；配置方法只验证不发送，发布成功挂钩使用原快照；重试校验 revision；永久删除清理配置；摘要和链接验证。
- [x] 界面：新增 Telegram 站点菜单和表单，保持保存/重试分离，清除 Token，阻止未保存配置误用；补 happy-dom 交互测试。
- [x] 集成：生成 MyGo 绑定，更新 README 与 CHANGELOG，运行 bun run check，独立审查改动并修复发现的问题。

## Verification notes

统一检查包含 Go 格式、go vet、前端测试、无缓存 Go 全套测试、TypeScript 和生产构建。浏览器使用模拟站点验证菜单、弹窗、保存状态和 Token 清空。独立审查发现并修复了导入文章 slug 和损坏配置无法清除的问题，均补回归测试。未运行真实 Telegram、在线 IPFS 或安装包验收。
