# Git 协作与版本管理

本仓库的主协作入口是 [GitHub](https://github.com/Stdq123/planet-windows)。`main` 保持可构建，通过短期分支和 Pull Request 合并改动。以下命令使用 PowerShell，在仓库根目录执行，另有说明的除外。

## 开始开发

先确认工作区没有需要保留的未提交改动，再更新主分支并创建任务分支：

```powershell
git status --short --branch
git switch main
git pull --ff-only origin main
git switch -c feature/short-description
```

按用途选择 `feature/`、`fix/`、`chore/` 或 `refactor/` 前缀。一个分支解决一个问题，尽量在一到三天内完成审查。已有未提交工作时先明确归属和保存方式，不要直接清理或重置。

涉及代码、依赖、配置或发行输入时，首次安装依赖，然后运行与 CI 一致的完整检查：

```powershell
Push-Location windows
bun install --frozen-lockfile
bun run check
Pop-Location
```

检查顺序为文档检查、Go 格式、`go vet`、前端测试、禁用结果缓存的 Go 测试、TypeScript 检查和 Vite 生产构建。任一步失败都会以非零状态退出。格式检查覆盖 Git 已跟踪和未忽略的新 Go 文件，不会自动改写代码；按提示对具体文件运行 `gofmt -w`。也可从根目录运行 `powershell -NoProfile -File scripts/check.ps1`。

检查需要 Git、Go 和 Bun；工具版本及 WebView2 要求见 [README](../README.md)。安装器构建与可选 Kubo、在线、模板测试独立执行，见 [Windows 指南](../windows/README.md)。本地默认检查通过不代表这些集成已经通过验收。

## 纯文档检查

仅修改以下普通文件时，本地 pre-push 与 CI 使用轻量检查：

- `README.md`、`windows/README.md`、`CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`、`SECURITY.md`。
- `docs/`、`windows/docs/` 下的 `.md` 文档，但不包括 `AGENTS.md`。
- `docs/screenshots/`、`docs/uiux/` 下的 `.png` 图片。
- `.github/PULL_REQUEST_TEMPLATE.md`。

在 `windows/` 运行 `bun run check:docs`，或在仓库根目录运行 `bun scripts/check-docs.ts`。只需 Git 和 Bun，无需 Go、前端依赖或构建。检查器读取所有已跟踪的人类文档的当前工作树内容，检查 UTF-8（无 BOM）、LF、结尾换行、冲突标记、代码围栏，以及文档中内联链接/图片指向的本地文件和标题锚点。代码示例中的链接会忽略，外部 URL 不联网验证。删除或改名后也会检查其他文档的入链；新文档需先暂存才能纳入手动检查。

`CHANGELOG.md` 是发行元数据输入，许可证、内置模板、应用资源及自动化配置也会影响构建或发布，因此这些文件即使看起来像文档，也执行完整检查。混合改动、文件模式变化、符号链接、空差异或无法确定比较基线时同样执行完整检查。范围判断集中在 `scripts/check-changes.ts`；`bun run check` 始终执行完整检查。

CI 保留 `Windows quality checks` 这一必需任务，每次 PR 和 `main` 推送都会运行；纯文档分支只执行文档检查，其他分支继续执行共享质量检查和依赖安全扫描。手动触发始终执行完整检查。任务内部进行分流，不使用工作流级 `paths-ignore`，避免[必需检查因工作流未触发而一直等待](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks)。PR 按实际待合并内容比较，`main` 推送按事件的前后提交比较。

## 本地 Git hooks

`pre-commit` 检查暂存区的空白错误，`pre-push` 按实际推送内容选择文档或完整质量检查。它们不会自动修改、暂存文件或安装依赖。新克隆需要自行启用；先检查已有 hook 配置，已有团队工具时将检查接入该工具，避免覆盖：

```powershell
git config --show-origin --get core.hooksPath
# 仅在没有现有 hook 配置时设置；作用范围为当前克隆。
git config --local core.hooksPath .githooks
```

hook 由 Git for Windows 执行，范围判断和文档检查使用 Bun，完整检查仍使用 Windows PowerShell。已有远端分支按 Git 传入的旧、新提交比较；新分支以对应远端 `main` 的共同祖先为基线，缺失时执行完整检查。多引用推送中任一更新需要完整检查就执行完整检查；标签使用完整检查，单纯删除引用不运行构建。

推送前应保持工作树干净，且所有待推送提交均对应当前检出的提交，否则 hook 会提示先保存改动或分别切换分支推送，避免检查了其他内容却放行本次推送。取消本仓库的本地设置可运行 `git config --local --unset core.hooksPath`，恢复继承的配置或 Git 默认 hooks。hooks 是本地辅助，服务端是否允许合并由仓库规则决定。

## 提交与审查

每个提交完成一个可解释、可验证的增量。使用 `git add -- <明确路径>` 或 `git add -p` 选择文件和代码块，提交前查看 `git diff --cached` 并运行 `git diff --cached --check`。锁文件应随依赖改动提交，生成的安装包、缓存和真实备份不入库。

提交标题使用 `<type>: <说明>`，必要时加正文解释原因：

| 类型 | 用途 |
| --- | --- |
| `feat` | 新功能 |
| `fix` | 修复行为 |
| `refactor` | 不改变行为的结构调整 |
| `test` | 测试 |
| `docs` | 文档 |
| `style` | 纯格式调整 |
| `chore` | 工具、配置、依赖和 CI |

例如 `fix: preserve the current article after a failed refresh`。一个提交只做一件事；纯格式调整与功能修改分开，不为了修饰已有公共提交重写历史。

完成检查后推送任务分支并在 GitHub 创建 PR。使用仓库 PR 模板说明问题、修改后的行为、检查结果和限制。新增可见行为时同步更新 [CHANGELOG](../CHANGELOG.md) 的 `Unreleased`，按 Added、Changed、Fixed、Deprecated、Removed、Security 分类。破坏兼容性的改动写出迁移方法。

Windows CI 在面向 `main` 的 PR 和 `main` 推送时按上述范围执行检查，也保留手动触发。同一分支的新提交会取消旧任务，单个任务最多运行 30 分钟。PR 中记录本地 `bun run check:docs` 或 `bun run check` 的适用结果，并等待 `Windows quality checks` 成功。Dependabot 版本检查每周执行，每生态最多保留 3 个版本更新 PR；安全更新提议也已开启，自动 rebase 和自动合并保持关闭。更新提议需要正常审查及本地检查；Bun 与 MyGo 相关包升级需要检查兼容性。依赖支持参见 [Dependabot 文档](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)。

GitHub ruleset 要求 `main` 通过 PR 合并、解决审查讨论并禁止非快进推送及删除，管理员保留应急通道。当前单人维护不要求独立批准；首次云端运行成功后，将 `Windows quality checks` 设为必需检查，并要求分支与 `main` 保持同步。配置记录和已核验状态见 [维护标准](maintenance.md)。

## 版本与发布

程序版本来源是 `windows/release.json`，Go 程序、MyGo 配置与前端共享该文件。发布记录由不可移动的 `v<版本>` 标签确定，标签必须与该提交中的完整版本和 Changelog 标题一致，包括 `-alpha.1` 等预发布后缀。版本主体为三个无前导零的 uint32 数字，可带 SemVer 预发布标识，不支持 `+` 构建元数据。配置中出现版本号不等于已经发布，不给历史快照补造标签或日期。

当前首个公开预发布目标为 `v0.0.1-alpha.1`；此前 `0.3.0` 等编号属于未打发行标签的开发快照，历史记录保持原样。这次重新确定公开编号不提供旧开发安装的降级迁移；alpha 先在隔离的新 Windows 用户环境测试。所选签名策略、许可或原生验收尚未满足发行要求时，变更继续保留在 `Unreleased`，不新增已发布条目。

采用语义化版本：修复通常递增 PATCH，兼容的新功能递增 MINOR，破坏已有兼容承诺的改动递增 MAJOR。当前仍为 `0.x` 开发阶段；任何不兼容变化都应在记录中突出说明，并提供迁移说明，不把它隐藏在补丁版本中。

发布通过单独 PR 更新 `release.json`，整理 Changelog，并按 [本地发布流程](releasing.md) 检查元数据、构建、准备材料和完成原生 Windows 验收。程序版本、标签和最终安装包必须一致。稳定版要求有效 Authenticode 签名；维护者明确选择未签名预发布时，使用 `-Release -AllowUnsignedPrerelease` 并保留未签名提示、实际签名状态及 SHA-256 摘要。该选项不豁免许可和原生验收，也不接受无效签名；GitHub pre-release 标记本身不能替代这些检查。

先确认发布元数据与目标提交，创建本地附注标签：

```powershell
$releaseVersion = (Get-Content windows/release.json -Raw -Encoding UTF8 | ConvertFrom-Json).version
git tag -a "v$releaseVersion" -m "Release $releaseVersion"
```

从该标签对应的干净提交构建，完成本地发布流程中的严格检查和验收后，再运行 `git push origin "v$releaseVersion"`。不要在验收前推送标签或创建公开发布。

当前 CI 不会因标签自动发布安装包，也不会修改更新源。上传安装包与签名更新清单沿用 [更新发布指南](../windows/docs/update-publisher.md)。已发布标签不移动；发现问题后创建修复提交与新版本。撤销已合并的代码时，通过新分支执行 `git revert <commit>`、验证后提交 PR。

## 镜像与并行工作

Gitea、Codeberg 是手动同步的独立仓库。先在 GitHub 完成合并，检查 `git remote -v` 并获取镜像最新状态，确认没有独立提交后再同步；普通非强制推送被拒绝时先调查差异。

需要同时处理两个任务时，可从 `main` 创建独立工作区：`git worktree add ../planet-windows-topic -b feature/topic main`。每个工作区单独安装依赖。任务合并且没有未提交内容后，使用 `git worktree remove ../planet-windows-topic` 清理。
