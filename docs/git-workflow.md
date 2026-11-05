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

首次安装依赖，然后运行与 CI 一致的检查：

```powershell
Push-Location windows
bun install --frozen-lockfile
bun run check
Pop-Location
```

检查顺序为 Go 格式、`go vet`、前端测试、禁用结果缓存的 Go 测试、TypeScript 检查和 Vite 生产构建。任一步失败都会以非零状态退出。格式检查覆盖 Git 已跟踪和未忽略的新 Go 文件，不会自动改写代码；按提示对具体文件运行 `gofmt -w`。也可从根目录运行 `powershell -NoProfile -File scripts/check.ps1`。

检查需要 Git、Go 和 Bun；工具版本及 WebView2 要求见 [README](../README.md)。安装器构建与可选 Kubo、在线、模板测试独立执行，见 [Windows 指南](../windows/README.md)。本地默认检查通过不代表这些集成已经通过验收。

## 本地 Git hooks

`pre-commit` 检查暂存区的空白错误，`pre-push` 执行完整质量检查。它们不会自动修改、暂存文件或安装依赖。新克隆需要自行启用；先检查已有 hook 配置，已有团队工具时将检查接入该工具，避免覆盖：

```powershell
git config --show-origin --get core.hooksPath
# 仅在没有现有 hook 配置时设置；作用范围为当前克隆。
git config --local core.hooksPath .githooks
```

hook 由 Git for Windows 执行，检查脚本使用 Windows PowerShell。取消本仓库的本地设置可运行 `git config --local --unset core.hooksPath`，恢复继承的配置或 Git 默认 hooks。hooks 是本地辅助，服务端是否允许合并由仓库规则决定。

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

当前按维护者要求暂停云端构建 CI：工作流仅保留手动触发，远端工作流保持停用。PR 中记录本地 `bun run check` 的结果；恢复自动构建检查需要另行决定。Dependabot 版本及安全更新提议已获独立授权开启：版本检查每周执行，每生态最多保留 3 个版本更新 PR；自动 rebase 和自动合并保持关闭。更新提议需要正常审查及本地检查；Bun 与 MyGo 相关包升级需要检查兼容性。依赖支持参见 [Dependabot 文档](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)。

GitHub ruleset 已要求 `main` 通过 PR 合并、解决审查讨论并禁止非快进推送及删除，管理员保留应急通道。当前单人维护不要求独立批准；CI 暂停时不设置必需云端检查。恢复 CI 后先验证实际运行，再添加对应必需检查。配置记录和已核验状态见 [维护标准](maintenance.md)。

## 版本与发布

程序版本来源是 `windows/release.json`，Go 程序、MyGo 配置与前端共享该文件。发布记录由不可移动的 `vX.Y.Z` 标签确定，标签必须与该提交中的版本文件和 Changelog 标题一致。配置中出现版本号不等于已经发布，不给历史快照补造标签或日期。

采用语义化版本：修复通常递增 PATCH，兼容的新功能递增 MINOR，破坏已有兼容承诺的改动递增 MAJOR。当前仍为 `0.x` 开发阶段；任何不兼容变化都应在记录中突出说明，并提供迁移说明，不把它隐藏在补丁版本中。

发布通过单独 PR 更新 `release.json`，整理 Changelog，并按 [本地发布流程](releasing.md) 检查元数据、构建、准备材料和完成原生 Windows 验收。程序版本、标签和最终安装包必须一致。

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
