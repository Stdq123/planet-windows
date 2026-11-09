# 本地发布流程

`CHANGELOG.md` 是面向用户的主记录；GitHub 自动发布说明只补充 PR 和贡献者索引。云端 CI 执行质量检查与依赖安全扫描；本文的材料准备脚本不会安装程序、创建标签、推送或发布 GitHub Release。另有显式手动触发的原生安装器测试，实际安装操作仅在一次性 Windows 虚拟机中执行。

## 日常改动

每个用户可见的功能、修复或安全更新随 PR 写入 `Unreleased`，说明实际影响。维护工作可按对贡献者的影响记录。准备发布时通过独立 PR 更新 `windows/release.json`；具备发行条件后，将对应条目移到 `## [版本] - YYYY-MM-DD` 下，填写实际发布日期并保留 `Unreleased` 标题。版本可含预发布后缀，例如 `0.0.1-alpha.1`。发行条目必须包含具体变化，日期不能在未来；仍有发行阻碍时保留 `Unreleased`，不预填发布日期。

`v0.0.1-alpha.1` 已公开，其[历史验收记录](acceptance-0.0.1-alpha.1.md)、标签与附件保持原样。维护者本次要求基于最新 `main` 重新打包并发布新 Logo 安装包，因此当前目标递增为 `v0.0.1-alpha.2`，版本文件为 `0.0.1-alpha.2`。本次明确按延续未签名预发布策略与七场景有限验收范围的假设执行，详见[本次验收记录](acceptance-0.0.1-alpha.2.md)；新包必须取得本次最终字节的新证据。

此前的 `0.1`、`0.2` 和 `0.3.0` 是未打发行标签的开发快照。已有 `0.2.0`/`0.3.0` 开发安装会拒绝降级到本 alpha；测试请使用隔离的新 Windows 用户环境，不覆盖原安装或用户资料。开发快照迁移及 alpha.1 到 alpha.2 的真实跨版本资料兼容仍需单独验证。

应用、更新发布工具和安装器接受三个无前导零的 uint32 数字及可选 SemVer 预发布后缀，不接受 `+` 构建元数据。预发布标识只能包含 ASCII 字母、数字、连字符和分隔用的点，纯数字标识不能有前导零。更新器与安装器按同一规则比较，例如 `0.0.1-alpha.2 < 0.0.1-alpha.10 < 0.0.1`。已安装稳定版的更新检查跳过预发布；预发布版可以接收更高版本的预发布或稳定版。MyGo 保留完整公开版本字符串，Windows PE 数字版本资源则由基础版本派生，例如 `0.0.1.0`，不能用它区分 alpha 次序。增加已公开版本的构建必须使用新版本号，避免同版本不同安装包。

## 检查与构建

在 `windows/` 下执行：

```powershell
bun install --frozen-lockfile
bun run check
bun run security:check
bun run build
```

`security:check` 在本机查询在线漏洞库；网络失败也会使检查失败。它检查 Bun 锁文件和 Go 源码中的可达漏洞，不覆盖打包的 Kubo 二进制和原版模板静态资源。第三方组件的版本、许可和安全情况仍需单独审查。

构建会打印独立发行目录，使用该目录准备草稿材料：

```powershell
$bundle = Read-Host '粘贴构建打印的独立发行目录'
bun run release:prepare -BundleDir $bundle
```

输出保存在 `.mygo/release-drafts/<版本>-<唯一编号>/`：安装包副本、`SHA256SUMS.txt`、`release-notes.md`、`audit.json` 和 `release-manifest.json`。默认模式明确标记 DRAFT，记录工作区是否有未提交改动，允许保留未签名和许可待审状态用于审查，不能据此对外发行。

每次输出使用新目录，不覆盖原包。准备时重新核对安装包和独立 payload 的一致性，复制后再次核对摘要。清单记录的 `sourceCommitAtPreparation` 仅表示准备材料时的检出版本，不是证明二进制由该提交构建的供应链证明。

## 对应源码与原生安装测试

正式材料必须同时携带从同一干净提交生成的对应源码包：

```powershell
powershell -NoProfile -File ../scripts/prepare-source.ps1
$sourceManifest = Read-Host '粘贴脚本输出的 source-manifest.json 完整路径'
```

记录脚本输出的 `source-manifest.json` 路径，作为后续 `-SourceManifest` 参数。源码包包含项目的已提交文件、固定版本的 Go vendor 源码、完整 go-ethereum 模块源码及许可材料，详见[源码与重建说明](rebuilding-release.md)。正式准备会核对提交、版本、文件名、大小和 SHA-256，并将源码包及清单复制到发行目录，加入 `SHA256SUMS.txt`；GitHub 自动生成的源代码 ZIP 不能替代这份带依赖的材料。

手动运行 [Native Windows installer smoke](../.github/workflows/windows-installer-smoke.yml) 会在 GitHub 托管的 Windows 虚拟机中完成共享检查、安全扫描、构建与真实静默安装测试。保留的 JSON 记录实际安装包 SHA-256、新装、同包重装、锁定和篡改拒绝、卸载及合成数据保留结果。只把该次通过的原始安装包与该证据关联；重新构建产生不同摘要时须重新测试。

该测试不覆盖 Windows 10/11 桌面界面、缺少 WebView2、中文输入、跨版本数据兼容、进程中断或断电恢复。稳定版仍须完成完整的适用验收；本次 `v0.0.1-alpha.2` 按下述有限范围重新验证，并保留全部未测项目，不沿用 alpha.1 的通过结果。脚本默认拒绝普通宿主；手动使用 `-IsolatedWindowsVM` 只适用于已确认干净且可丢弃的虚拟机。

## 对外发行检查（含 alpha）

默认发行要求程序和安装包具有有效 Authenticode 签名，稳定版始终遵循此要求。维护者明确选择未签名预发布时，可使用下述 `-AllowUnsignedPrerelease` 选项；GitHub 的 pre-release 标记本身不会启用此例外。许可和适用的原生 Windows 验收要求仍须满足；本次 alpha.2 的发布请求、执行假设及有限范围见下节，不自动适用于其他版本。

1. 完成发布 PR，在干净工作区检查目标版本条目：

   ```powershell
   $version = (Get-Content release.json -Raw -Encoding UTF8 | ConvertFrom-Json).version
   bun run release:check --tag "v$version"
   ```

2. 确认本地 HEAD 为待发布提交后，创建对应的附注标签，先不推送。重新从该提交执行检查和构建。`release:check` 只检查元数据；标签是否真实存在由下一步的正式准备检查。
3. 对于签名发行，用受信任的发布身份签署第一方程序，再生成包含已签名程序的安装包，最后签署安装包。现有构建脚本尚未配置证书和自动签名环节，不能假定 `bun run build` 的输出已经签名。明确选择未签名预发布时，按[未签名预发布](#未签名预发布)一节记录签名状态。两种情况都要重新运行 [安装包审计](../windows/docs/release-audit.md) 验证最终字节。
4. 填写 [原生 Windows 验收记录](release-acceptance-template.md)，关联最终安装包 SHA-256；本次 alpha.2 采用[已披露的有限验收范围](acceptance-0.0.1-alpha.2.md)，最终结果作为 Release 附件。完成第三方许可审查。使用生产更新源时，另行完成[更新清单验证](../windows/docs/update-publisher.md)；本次 alpha 没有配置生产更新源，只提供手动下载，生产 feed 发布步骤不适用，不承诺自动更新。这些适用步骤需要实际环境和证据。
5. 执行严格的本地材料检查：

   ```powershell
   bun run release:prepare -BundleDir $bundle -SourceManifest $sourceManifest -Release -Tag "v$version"
   ```

严格模式要求版本与标签相同、存在有效日期的 Changelog、干净工作区、附注标签指向 HEAD、安装包 payload 一致、对应源码清单和摘要匹配、程序及安装包各有一条 Authenticode 审计记录、Kubo 许可待审计数为零。五个内置模板继续保留，本项目按维护者决定移除其待审发布门槛；上游来源、署名及已有许可文本保持保留。默认仅接受 `Valid` 签名状态；显式例外见[未签名预发布](#未签名预发布)。任何失败都不会被当作通过；保留未完成输出用于排查，勿将其上传。

技术检查不能代替完整许可判断、签名发行的发布者证书身份确认、适用的原生 Windows 验收及备份兼容性验证。全部适用验收通过后，维护者再推送标签、创建 GitHub Release 草稿，核对材料后发布；预发布必须标记为 pre-release。未测试的备份格式不能声明兼容。脚本不会自动执行这些外部操作。

## 本次 alpha 的有限验收范围

alpha.1 的有限范围决定及候选证据保留在[历史记录](acceptance-0.0.1-alpha.1.md)，不改写为 alpha.2。维护者本次新请求是重新打包并发布含新 Logo 的最新主分支版本；`v0.0.1-alpha.2` 按继续未签名预发布及七场景有限验收的明确假设执行。此范围仅限本次发布请求，不成为后续版本默认豁免。

本次必须重新测试最终包的七个原生静默安装场景，同时公开 Windows 10/11 GUI、WebView2、中文输入、真实跨版本升级、进程中断与断电恢复、真实 Mac 备份及其他未测项目。完整范围保存在[alpha.2 验收记录](acceptance-0.0.1-alpha.2.md)，不得删除未测项目或将其标为通过；稳定版仍须完成完整的适用原生验收和有效签名。

先前成功运行只证明其记录中的提交及安装包，不能挪用到本次发行。发布 PR 合并后，从最终附注标签所指向的干净提交重新执行质量检查、安全扫描、构建、安装包审计及七场景原生安装测试，并生成和验证对应源码包及重建结果。最终安装包必须与本次最终原生证据中的 SHA-256 一致。将最终提交、运行链接、安装包及源码包摘要、原生 JSON 和本次限制说明作为 GitHub Release 附件保存；不在当前提交中预填尚未生成的包摘要，也不回写自引用的最终标签提交 SHA。历史和候选记录保留其原始身份。

本次继续保留全部内置模板及其来源、署名和已有许可文本，不重新设置模板待审门槛。本次按延续未签名策略的假设执行，签名服务申请或等待状态不作为本次 alpha 的阻碍，也不作为已签名证据；未来接入代码签名后，以最终 Authenticode 验证结果为准。

## 未签名预发布

维护者明确选择未签名 alpha 等预发布后，在完成其余发行检查的前提下，使用：

```powershell
bun run release:prepare -BundleDir $bundle -SourceManifest $sourceManifest -Release -Tag "v$version" -AllowUnsignedPrerelease
```

此参数必须与 `-Release` 同时使用，且版本必须带预发布后缀；稳定版和草稿模式不能使用。程序及安装包各自的 Authenticode 状态只能是 `Valid` 或 `NotSigned`。缺失或重复记录、`HashMismatch`、`NotTrusted`、`UnknownError` 等其他状态仍会阻止发行；元数据、标签、payload、许可审查和适用的原生 Windows 验收要求保持不变。本次 alpha 的有限验收范围来自维护者的单独决定，不由该参数自动豁免。

只要程序或安装包有一项未签名，材料会记录 `unsignedPrerelease: true` 和实际签名状态，并在发行说明中加入 **UNSIGNED PRERELEASE** 提示。GitHub Release 必须保留该提示，同时提供 `SHA256SUMS.txt` 和审计材料。SHA-256 只能核对文件是否与所列摘要一致，不能证明发布者身份；Windows 可能显示安全提示或阻止运行。不要把未签名包描述成已验证发布者身份的包，也不要要求使用者关闭系统防护。

申请代码签名服务不代表已获批准或已经签名。后续接入签名服务时，以最终程序和安装包的实际 Authenticode 审计结果为准。

## 出现问题时

暂停推广问题版本及更新源。先保存安装包摘要、运行版本、错误信息和本机恢复证据，避免收集私钥或完整备份。安装中断按 [升级与恢复](../windows/docs/safe-upgrade.md) 使用同一包重试；资料库格式变更后不能直接换旧程序或覆盖资料目录。修复通过新版本发行，不移动已发布标签、不替换同版本安装包。

GitHub 的说明分类由 [release.yml](../.github/release.yml) 提供，使用 `breaking-change`、`enhancement`、`bug`、`dependencies` 等标签；维护者应先审查内容。参见 [GitHub 发布说明文档](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes)。
