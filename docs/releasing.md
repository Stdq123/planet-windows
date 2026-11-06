# 本地发布流程

`CHANGELOG.md` 是面向用户的主记录；GitHub 自动发布说明只补充 PR 和贡献者索引。云端 CI 执行质量检查与依赖安全扫描；本文的发行构建和材料准备在本机执行。脚本不会安装程序、创建标签、推送或发布 GitHub Release。

## 日常改动

每个用户可见的功能、修复或安全更新随 PR 写入 `Unreleased`，说明实际影响。维护工作可按对贡献者的影响记录。准备发布时通过独立 PR 更新 `windows/release.json`；具备发行条件后，将对应条目移到 `## [版本] - YYYY-MM-DD` 下，填写实际发布日期并保留 `Unreleased` 标题。版本可含预发布后缀，例如 `0.0.1-alpha.1`。发行条目必须包含具体变化，日期不能在未来；仍有发行阻碍时保留 `Unreleased`，不预填发布日期。

当前目标是首个公开预发布 `v0.0.1-alpha.1`，版本文件为 `0.0.1-alpha.1`。此前的 `0.1`、`0.2` 和 `0.3.0` 是未打发行标签的开发快照；本次重新确定公开版本编号，不改写历史验收记录，也不表示 alpha 已获准发布。已有 `0.2.0`/`0.3.0` 开发安装会拒绝这次降级；测试 alpha 请使用隔离的新 Windows 用户环境，不要覆盖原安装或用户资料。开发快照到公开版本的迁移另行验证。

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

## 对外发行检查（含 alpha）

默认发行要求程序和安装包具有有效 Authenticode 签名，稳定版始终遵循此要求。维护者明确选择未签名预发布时，可使用下述 `-AllowUnsignedPrerelease` 选项；GitHub 的 pre-release 标记本身不会启用此例外。许可和原生 Windows 验收要求不变，存在这些未决项时仍只能准备本地草稿。

1. 完成发布 PR，在干净工作区检查目标版本条目：

   ```powershell
   $version = (Get-Content release.json -Raw -Encoding UTF8 | ConvertFrom-Json).version
   bun run release:check --tag "v$version"
   ```

2. 确认本地 HEAD 为待发布提交后，创建对应的附注标签，先不推送。重新从该提交执行检查和构建。`release:check` 只检查元数据；标签是否真实存在由下一步的正式准备检查。
3. 对于签名发行，用受信任的发布身份签署第一方程序，再生成包含已签名程序的安装包，最后签署安装包。现有构建脚本尚未配置证书和自动签名环节，不能假定 `bun run build` 的输出已经签名。明确选择未签名预发布时，按下一节记录签名状态。两种情况都要重新运行 [安装包审计](../windows/docs/release-audit.md) 验证最终字节。
4. 填写 [原生 Windows 验收记录](release-acceptance-template.md)，关联最终安装包 SHA-256；完成第三方许可审查及 [更新清单验证](../windows/docs/update-publisher.md)。这些步骤需要实际环境和证据。
5. 执行严格的本地材料检查：

   ```powershell
   bun run release:prepare -BundleDir $bundle -Release -Tag "v$version"
   ```

严格模式要求版本与标签相同、存在有效日期的 Changelog、干净工作区、附注标签指向 HEAD、安装包 payload 一致、程序及安装包各有一条 Authenticode 审计记录、Kubo 许可待审计数为零。默认仅接受 `Valid` 签名状态；未签名预发布的显式例外见下节。任何失败都不会被当作通过；保留未完成输出用于排查，勿将其上传。

技术检查不能代替完整许可判断、签名发行的发布者证书身份确认、原生 Windows 验收及备份兼容性验证。全部适用验收通过后，维护者再推送标签、创建 GitHub Release 草稿，核对材料后发布；预发布必须标记为 pre-release。脚本不会自动执行这些外部操作。

## 未签名预发布

维护者明确选择未签名 alpha 等预发布后，在完成其余发行检查的前提下，使用：

```powershell
bun run release:prepare -BundleDir $bundle -Release -Tag "v$version" -AllowUnsignedPrerelease
```

此参数必须与 `-Release` 同时使用，且版本必须带预发布后缀；稳定版和草稿模式不能使用。程序及安装包各自的 Authenticode 状态只能是 `Valid` 或 `NotSigned`。缺失或重复记录、`HashMismatch`、`NotTrusted`、`UnknownError` 等其他状态仍会阻止发行；元数据、标签、payload、许可审查和原生 Windows 验收要求保持不变。

只要程序或安装包有一项未签名，材料会记录 `unsignedPrerelease: true` 和实际签名状态，并在发行说明中加入 **UNSIGNED PRERELEASE** 提示。GitHub Release 必须保留该提示，同时提供 `SHA256SUMS.txt` 和审计材料。SHA-256 只能核对文件是否与所列摘要一致，不能证明发布者身份；Windows 可能显示安全提示或阻止运行。不要把未签名包描述成已验证发布者身份的包，也不要要求使用者关闭系统防护。

申请代码签名服务不代表已获批准或已经签名。后续接入签名服务时，以最终程序和安装包的实际 Authenticode 审计结果为准。

## 出现问题时

暂停推广问题版本及更新源。先保存安装包摘要、运行版本、错误信息和本机恢复证据，避免收集私钥或完整备份。安装中断按 [升级与恢复](../windows/docs/safe-upgrade.md) 使用同一包重试；资料库格式变更后不能直接换旧程序或覆盖资料目录。修复通过新版本发行，不移动已发布标签、不替换同版本安装包。

GitHub 的说明分类由 [release.yml](../.github/release.yml) 提供，使用 `breaking-change`、`enhancement`、`bug`、`dependencies` 等标签；维护者应先审查内容。参见 [GitHub 发布说明文档](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes)。
