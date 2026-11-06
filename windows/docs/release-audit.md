# Windows 发行包检查与隔离安装测试

本流程检查安装包内容，另在临时 Docker 容器中的 Wine 环境测试安装和卸载。Wine 测试不能代替干净 Windows 10/11 虚拟机、WebView2、图形界面、签名以及升级兼容性验收。

`v0.0.1-alpha.1` 的原生安装结果、明确接受的已知限制与最终附件要求见 [alpha 验收记录](../../docs/acceptance-0.0.1-alpha.1.md)。下文标注日期的开发包测试属于历史记录，不是本次最终发行包的验收身份。

## 构建和静态校验

当前已审查的 MyGo Go 模块及 CLI 版本为 `0.7.4`。上游 [NSIS 模板](https://github.com/egoist/mygo/blob/v0.7.4/cmd/mygo/nsis.go) 仍在 `customUnInstall` 后执行 `RMDir /r "$INSTDIR"`，现有 hook 的目录重定向保护继续适用。升级只更新精确版本检查，不移除重新审查要求；最终交付包仍由本仓库的安全安装器脚本构建。

2026-11-05 的升级验证包括：冻结锁文件安装、生成绑定无差异、本地安全扫描、Windows 原生构建、10 项安装内容哈希一致，以及合成安装包的正常接受/篡改拒绝测试。隔离资料库中的真实窗口通过启动、中文表单输入、本地站点创建和退出检查；测试数据未提交。检查应用互斥锁的单元测试应在测试应用退出后执行。

该次开发安装包 SHA-256 为 `1d4a8cb66be5e34b6c1dd7d18bb3c2d9fd89f479a5418cbf928251f71e9c683e`。这次升级不是正式发布；当时仍存在未签名、Kubo 许可材料缺项和完整原生安装/升级/卸载验收未完成的限制。当前 Kubo 材料范围见下文，alpha 的验收范围见上述独立记录。

在 `windows/` 目录运行 `bun run build`，构建结束会打印唯一发行目录 `.mygo/releases/<GUID>/windows-amd64/`，检查通过后还会同步到固定的 `build/windows-amd64/`；两个目录均包含独立安装器和所需 payload。MyGo 默认生成的安装器因进程关闭和目录处理行为不能作为交付包。`prepare-kubo.ps1` 验证固定 Kubo 包的 SHA-512；MyGo 的前端构建命令会调用 `prepare-notices.ps1`，从 Windows/amd64 构建依赖及 `mygo-runtime` 生成 `LICENSE-PLANET.txt`、`THIRD-PARTY-NOTICES.txt` 和 `KUBO-BUILDINFO.txt`，并校验及复制 `KUBO-DEPENDENCY-LICENSES.txt`。

```powershell
$releaseDir = Read-Host '粘贴构建结束时显示的发行目录'
powershell -NoProfile -File scripts/audit-release.ps1 -BundleDir $releaseDir
```

该命令要求本机 `C:\Program Files\7-Zip\7z.exe`；可用 `-SevenZip` 指定其他路径。它通过只读解压流计算安装包条目 SHA-256，逐个比较同目录的程序及许可文件，并输出 Authenticode 状态。出现缺失或额外 payload、重复条目、解压错误或内容不一致时以非零状态退出。签名状态仅供人工判定，`NotSigned` 不是已签名。脚本不会运行安装程序或改动注册表。

7-Zip 24.09 不列出本安装包中由 NSIS `WriteUninstaller` 生成的卸载器，26.05 会显示 `Uninstall.exe` 及 `$PLUGINSDIR\update-payload\Uninstall.exe`。审计兼容两份均不可见的旧格式；可见时要求两份各一条且解压流摘要一致。该规则仅对应安装脚本已定义的卸载器，其他未知根目录条目仍会失败。云端构建失败时，手动安装测试工作流会保留失败包和 7-Zip 清单供诊断；失败材料不是可发布安装包。

可用 `powershell -NoProfile -File scripts/test-release-audit.ps1` 构造一个仅含测试文本的 NSIS 文件，验证未改变的文件被接受、修改后的 payload 被拒绝；需要 MyGo 缓存中的 NSIS 3.13 和 7-Zip。这个测试不运行合成安装器，临时测试文件保留在 `.mygo/release-audit-tests/`。

## 隔离安装、覆盖安装和卸载

首次准备 Docker 镜像需要访问 Debian 软件包仓库并占用约 2.3 GB 或更多镜像空间：

```powershell
docker build --tag planet-release-audit:local scripts/release-audit
$releaseDir = Read-Host '粘贴构建结束时显示的发行目录'
$installer = Join-Path $releaseDir 'planet-windows-safe-setup-0.2.0-amd64.exe'
powershell -NoProfile -File scripts/test-installer-isolated.ps1 -Installer $installer -SafeChecks
# 可选：验证新版拒绝覆盖指定的旧安装包。
powershell -NoProfile -File scripts/test-installer-isolated.ps1 -Installer $installer -SafeChecks -PreviousInstaller 'build/phase11/windows-amd64/planet-windows-setup-0.1.0-amd64.exe'
```

执行安装器的容器禁用网络，根文件系统只读，Wine 前缀与注册表位于容器内临时文件系统；安装包和测试脚本均只读挂载。Wine 8 缺少 Go 运行时需要的系统 DLL，因此镜像内编译了只供测试使用的随机数兼容层；该 DLL 仅放入临时 Wine 前缀，不进入发行包。`-SafeChecks` 检查冲突目录、同名文件、其他进程、锁与修改过的文件、快捷方式以及保留用户数据。指定旧包时检查拒绝覆盖该包，不能证明旧版数据迁移。

独立安装器的目标是避免 MyGo 自动生成安装器中的强制关闭进程和递归删除；其守卫拒绝非空目录、新装时发现的已有注册以及路径中的重解析点，卸载前核对登记的哈希和文件锁。哈希计算期间持有独占句柄，但守卫返回到 NSIS 逐个删除文件之间仍存在并发替换或加锁的时间窗口；删除中途失败可能已删除部分自有文件，卸载入口在通常失败路径下保留以便重试，注册表或恢复写入也失败时不能保证入口仍可用。`record` 写注册表失败后会尝试撤销新建的不完整注册表状态，但已复制的文件可能留在安装目录；该目录此时不再为空，不能直接用安装器原路径重试。请在独占可控的环境操作，不要把它当作针对恶意并发修改的安全边界。仅当现有安全安装的所有登记文件、路径及其哈希与当前安装包完全一致时，重复运行会只读成功退出；协议 2 的较新安装包可执行可重试的原地升级，见 [升级与恢复说明](safe-upgrade.md)；旧安全 0.1 与 MyGo 安装器创建的版本仍拒绝覆盖。新增随包资源时必须同步更新独立安装器、卸载清单、测试和 `audit-release.ps1`；旧安装器不会自动获得修复。

## 许可和发布限制

生成脚本收集主程序实际链接的 Go 模块根目录中名为 LICENSE、COPYING、NOTICE 的材料、Go 标准库及 `mygo-runtime` 许可。`KUBO-BUILDINFO.txt` 列出 Kubo 二进制中的模块版本；`KUBO-DEPENDENCY-LICENSES.txt` 根据 Kubo 的编译元数据逐一下载模块、核对版本与 h1，收集许可材料。当前 241 个模块均有材料，未解决的收集缺项为零。

其中四项没有根目录许可文件的模块使用严格限定的补充证据：`ipfs/bbloom` 的源码 MIT 与 CC0 声明、`p2p-forge/client` 的嵌套许可证，以及 `goissue34681`、`base32` 的 Go Authors 声明与对应上游 Go BSD 文本。补充规则固定模块版本、h1、源码修订及文件 SHA-256；`p2p-forge` 规则还绑定实际只链接 client 的官方 Kubo 0.38.2 二进制。版本或文件变化不能沿用该结论。重新收集可运行 `go run scripts/release-audit/kubo-notices.go -output docs/kubo-dependency-licenses.txt`，需要网络；存在缺项时返回非零并保留证据文件。

五个内置模板保留原固定版本、来源、署名和已有许可文本；按维护者决定，本项目不再以模板待审状态阻止 alpha 发布，不因此替上游授予新的许可。go-ethereum 库使用 LGPL-3.0，对应源码与重建方法见[源码与重建说明](../../docs/rebuilding-release.md)。这些材料说明收集范围，不代表嵌套子目录、前端其他文件或各项许可证义务均已完成全面法律审查。

发布前还需保留适用的第三方许可材料与对应源码、按选定签名策略验证、在干净的 Windows 系统中检查新装/跨版本升级/卸载与数据保留，并检验安装包实际内容。以上 Wine 测试不会覆盖这些事项。
