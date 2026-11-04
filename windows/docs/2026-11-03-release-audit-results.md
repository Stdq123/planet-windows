# 发行检查记录：2026-11-03

本记录针对当前工作副本，不代表未来构建或原生 Windows 安装验收通过。整个安装器执行测试使用 Docker 内的独立 Wine 前缀和注册表；测试容器禁网，安装包和测试脚本为只读挂载，未运行任何安装器于宿主 Windows。

## 最终独立安装包复验

最终包 `.mygo/releases/2ef5fa4433a648d091208780bc363d5b/windows-amd64/planet-windows-safe-setup-0.1.0-amd64.exe` 的 SHA-256 为 `a210f688246c20fe84fdddabab28c051e263614f02ad282fbdfbc9cef8e20bf2`。它使用独立安装器，下面的 MyGo 卸载钩子样例只保留为早期调查记录。

该包在禁网、只读挂载安装包的 Docker/Wine 临时环境中通过新装、正常卸载、共享目录和同名文件拒绝、快捷方式保护、锁定或修改文件时拒绝卸载、用户数据保留和旧 MyGo 安装的升级拒绝测试。它不支持自动升级；拒绝旧安装时，旧文件及注册项保持不变。当前原生 Windows 守卫单测覆盖重解析点祖先与独占文件锁；原生 Windows 安装器端到端、检查后并发替换竞态、失败后的自动恢复和签名仍未验证或未完成。

## 发行资源与许可

- 检查 `build/windows-amd64/planet-windows-setup-0.1.0-amd64.exe` 的 7-Zip NSIS 列表：旧包包含主程序、`ipfs.exe`、图标和 Kubo 三份许可，但缺少 Planet 许可及第三方许可汇总。该旧包在增加卸载钩子之前生成，不能据此推断它有卸载保护。
- `Get-AuthenticodeSignature` 对旧包、主程序及 Kubo 程序均返回 `NotSigned`。
- `powershell -NoProfile -File scripts/prepare-notices.ps1` 成功收集了 Windows/amd64 主程序的 51 个 Go 模块根目录许可及 mygo-runtime 许可；两次运行的输出 SHA-256 相同。
- `go run scripts/release-audit/kubo-notices.go -output docs/kubo-dependency-licenses.txt` 读取 Kubo 二进制中的 241 个依赖，逐项下载匹配版本并比对 h1；237 个找到根目录许可/声明，4 个未找到，命令按设计返回非零并保留证据。在依赖就绪后再次生成，证据文件 SHA-256 一致。缺失模块为 `github.com/alexbrainman/goissue34681`、`github.com/ipfs/bbloom`、`github.com/ipshipyard/p2p-forge` 和 `github.com/whyrusleeping/base32`。有些源码包含许可头或包元数据，仍需逐项判断；不能据此假定所有义务已满足。
- `powershell -NoProfile -File scripts/test-release-audit.ps1`：合成 NSIS 中的原始 payload 校验通过；修改旁边的模拟 `ipfs.exe` 后被哈希检查拒绝。实际旧包因缺少 `LICENSE-PLANET.txt` 被该脚本拒绝。

## 卸载保护：样例正反验证

在 Windows 编译同构测试包，输出文件位于 `.mygo/release-audit-tests/`，脚本本身只创建模拟程序：

```powershell
$compiler = Join-Path $env:LOCALAPPDATA 'mygo/nsis-3.13/makensis.exe'
$testRoot = Join-Path (Get-Location) '.mygo/release-audit-tests'
New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
$hook = Join-Path (Get-Location) 'scripts/installer-safe-uninstall.nsh'
$safe = Join-Path $testRoot 'hook-harness.exe'
$unsafe = Join-Path $testRoot 'hook-harness-unsafe.exe'
& $compiler "/DOUTPUT=$safe" "/DHOOK=$hook" scripts/release-audit/hook-harness.nsi
& $compiler /DUNSAFE "/DOUTPUT=$unsafe" scripts/release-audit/hook-harness.nsi
powershell -NoProfile -File scripts/test-installer-isolated.ps1 -Installer $safe
powershell -NoProfile -File scripts/test-installer-isolated.ps1 -Installer $unsafe
```

正例在隔离 Wine 中输出：`PASS: silent install into a pre-existing directory`、`PASS: repeat install preserved unrelated file and user data`、`PASS: uninstall preserved unrelated files and user data while removing owned payload`。反例不加载钩子，卸载时输出 `FAIL: uninstall removed unrelated file` 并以非零退出，表明测试能够发现原有递归删除风险。

将同构的不安全样例作为旧版、含钩子的样例作为新版，通过 `test-installer-isolated.ps1 -Installer $safe -PreviousInstaller $unsafe` 验证后输出 `PASS: install over previous version preserved unrelated file and user data`，并在随后卸载时保留用户文件。此项仅为测试器和钩子的合成版本验证，并不代表真实的跨版本安装已通过。

该样例不证明完整真实安装器安全：尚需对包含钩子的新构建实际安装包执行同一测试，并在隔离原生 Windows 环境复验新装、跨版本升级与卸载。任何安装、升级或卸载不得以真实用户数据或工作目录作为测试对象。
