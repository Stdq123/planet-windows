# 项目维护标准

## 当前执行方式

维护者已授权恢复 Windows CI。`Windows checks` 在面向 `main` 的 PR 和 `main` 推送时始终运行 `Windows quality checks`：仅修改[文档白名单](git-workflow.md#纯文档检查)时执行 `bun run check:docs`，其他改动执行 `bun run check` 与 `bun run security:check`。手动触发或无法可靠判断范围时执行完整检查。工作流使用只读权限和固定 Action SHA，同一分支的新提交取消旧任务，单个任务超时为 30 分钟。`Windows quality checks` 保持为 `main` 的必需检查，在任务内部进行分流，不通过工作流路径过滤跳过检查，避免 PR 永久等待。

仓库级 Actions 与 Windows 工作流均开启。Dependabot 对 GitHub Actions、Go modules 和 Bun 每周检查版本，各生态最多保留 3 个版本更新 PR；安全更新自动 PR 对受支持的依赖启用，采用独立于版本更新的数量限制。自动 rebase 保持禁用，不配置自动合并；所有依赖升级仍需本地验证、云端检查和人工合并。

Dependabot 使用标准 GitHub 托管 runner 时不消耗套餐内的 Actions 分钟数，参见 [GitHub 官方说明](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependabot-on-actions)。历史上合并基础配置时曾意外启动 6 个依赖更新任务，随后暂停；现按维护者新指令恢复，保留该历史记录。

维护入口是 [Git 协作指南](git-workflow.md)、[本地发布流程](releasing.md) 和 [验收模板](release-acceptance-template.md)。维护人通过 [CODEOWNERS](../.github/CODEOWNERS) 路由；尚无第二名维护者时不要求无法完成的独立批准。

## GitHub 服务端状态（2026-11-05 核验）

| 项目 | 状态 |
| --- | --- |
| 默认分支规则 | 已启用 PR、解决审查讨论、禁止删除及非快进推送 |
| 版本标签规则 | `v*` 标签禁止更新与删除 |
| 管理员应急通道 | 保留 bypass；仅用于有记录的应急处理 |
| 必需云端状态检查 | `Windows quality checks`，只接受 GitHub Actions 的结果，要求分支与 `main` 同步 |
| 密钥扫描及推送保护 | 已启用 |
| 私密漏洞报告 | 已启用 |
| CodeQL 默认配置 | 未配置；当前 CI 执行共享质量检查与依赖安全扫描 |
| Dependabot 版本更新 | 已启用，每周检查，每生态最多 3 个未关闭 PR |
| Dependabot 安全更新 | 已启用，对受支持的依赖自动提出修复 PR |
| 依赖 PR 自动合并 | 未启用，合并前完成本地验证 |

规则配置分别保存在 [main.json](../.github/rulesets/main.json) 和 [releases.json](../.github/rulesets/releases.json)，便于审查与恢复。它们是配置记录，修改文件本身不修改远端。管理员有应急绕过能力，因此标签不可变仍是维护承诺，不是对管理员的绝对限制。

## 每次改动与依赖维护

- 行为变化有相应测试，修复有能复现旧问题的回归用例；运行 `bun run check`，纯文档改动可使用 `bun run check:docs`。PR 填写适用命令、结果和未执行项目。
- 涉及依赖或准备发布时运行 `bun run security:check`。该检查需要访问 npm/Go 漏洞库；网络错误不能当作无漏洞。其扫描范围不含独立 Kubo 二进制和第三方模板资产。
- 依赖更新逐项或按必要的关联包处理，阅读上游变更和安全公告，核对锁文件变化，完成回归后提交。Dependabot 配置是更新提议入口，不配置自动合并。
- Actions 已固定到完整提交 SHA，保留只读默认权限。更新 SHA 时验证官方来源和对应版本，并由 Dependabot 提议后审查。
- 数据迁移、备份格式、安装器与签名变更属于重点审查项，更新对应恢复说明与验收证据。

## 发布前必须取得的外部证据

程序与安装器 Authenticode 签名、干净原生 Windows 的安装/升级/卸载、声明兼容的真实备份往返，以及完整第三方许可审查。当前 Kubo 许可清单仍有 4 项待审，现有构建未签名；工具检查通过不能消除这些发布阻断项。

发布后按版本与安装包摘要定位问题，保留旧版文件和迁移证据，记录故障复现与修复；数据格式升级后不承诺直接降级。问题报告先按 [安全政策](../SECURITY.md) 和隐私要求脱敏。

参考：[GitHub rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets)、[Actions 安全指南](https://docs.github.com/en/actions/reference/security/secure-use)、[Go 漏洞管理](https://go.dev/doc/security/vuln/)、[Bun audit](https://bun.com/docs/pm/cli/audit)。
