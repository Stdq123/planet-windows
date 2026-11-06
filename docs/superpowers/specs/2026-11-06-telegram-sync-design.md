# Telegram 文章同步

每个站点可绑定一个 Telegram 群组或频道。用户选择消息格式为「标题＋摘要＋文章链接」。在 Windows 项目现有 Go / MyGo / TypeScript 架构中实现。

## 方案

采用 Telegram Bot API：用户在本机填写 Bot Token 和目标 chat ID / @username，保存时通过 getChat 验证并固定数字 ID。沿用站点发布成功后的集成入口。RSS 转发依赖额外服务，用户账号登录引入不必要的会话管理，因此本次不采用。

## 行为

- 配置保存仅校验目标，不发送消息。首次配置后的下一次成功发布同步现有文章；以后只发送新增内容或更新原消息。独立页面、草稿不参与。
- 消息使用纯文本，摘要去除 Markdown / HTML 标记，限制长度；链接使用站点现有规范地址和文章 slug / ID。
- 配置和消息 ID / 内容指纹使用 Windows DPAPI 存在 integrations/telegram，独立于公开网站和备份，不向前端返回 Token。
- 同步使用已经成功发布的内存快照。失败单独警告，不改变 IPNS 发布成功结果。不确定的首次发送结果禁止自动重发；界面提供明确的手动重试。
- 手动同步仅允许当前保存版本已成功发布的站点，避免把未发布编辑同步出去。重复发布按实际消息指纹去重；更新原消息失败不偷偷新发。
- 每批最多处理 20 条变更，余下在后续发布或手动同步继续。限流时停止并反馈。
- 更换目标或机器人重置本机消息历史；相同机器人轮换 Token 保留历史。停用移除本机配置，永久删除站点清理配置。远程消息不自动删除。

## 验证

使用本地模拟 Telegram HTTP 响应与客户端，覆盖配置隔离、加密、令牌隐藏、发布快照、链接及摘要、重复跳过、更新、异常和重试、前端操作与状态。执行 bun run check。没有真实 Bot 凭据时不声称完成线上验收。

协议参考：[Telegram Bot API](https://core.telegram.org/bots/api#sendmessage)，[getChat](https://core.telegram.org/bots/api#getchat)，[editMessageText](https://core.telegram.org/bots/api#editmessagetext)。
