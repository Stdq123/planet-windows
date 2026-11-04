# Windows 发行包检查与隔离安装测试

本流程检查安装包内容，另在临时 Docker 容器中的 Wine 环境测试安装和卸载。Wine 测试不能代替干净 Windows 10/11 虚拟机、WebView2、图形界面、签名以及升级兼容性验收。

## 构建和静态校验

在 `windows/` 目录运行 `bun run build`，构建结束会打印唯一发行目录 `.mygo/releases/<GUID>/windows-amd64/`，检查通过后还会同步到固定的 `build/windows-amd64/`；两个目录均包含独立安装器和所需 payload。MyGo 默认生成的安装器因进程关闭和目录处理行为不能作为交付包。`prepare-kubo.ps1` 验证固定 Kubo 包的 SHA-512；MyGo 的前端构建命令会调用 `prepare-notices.ps1`，从 Windows/amd64 构建依赖及 `mygo-runtime` 生成 `LICENSE-PLANET.txt`、`THIRD-PARTY-NOTICES.txt` 和 `KUBO-BUILDINFO.txt`，并校验及复制 `KUBO-DEPENDENCY-LICENSES.txt`。

```powershell
$releaseDir = Read-Host '粘贴构建结束时显示的发行目录'
powershell -NoProfile -File scripts/audit-release.ps1 -BundleDir $releaseDir
```

该命令要求本机 `C:\Program Files\7-Zip\7z.exe`；可用 `-SevenZip` 指定其他路径。它通过只读解压流计算安装包条目 SHA-256，逐个比较同目录的程序及许可文件，并输出 Authenticode 状态。出现缺失或额外 payload、重复条目、解压错误或内容不一致时以非零状态退出。签名状态仅供人工判定，`NotSigned` 不是已签名。脚本不会运行安装程序或改动注册表。

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

生成脚本收集主程序实际链接的 Go 模块根目录中名为 LICENSE、COPYING、NOTICE 的材料、Go 标准库及 `mygo-runtime` 许可；它并未证明嵌套子目录、前端其他文件或各许可证义务均已完全覆盖。`KUBO-BUILDINFO.txt` 列出 Kubo 二进制中的模块版本；`KUBO-DEPENDENCY-LICENSES.txt` 根据 Kubo 的编译元数据逐一下载模块、核对版本与 h1，收集其根目录许可材料。当前 241 个模块中仍有 4 个未找到根目录许可文件，标记为 `REQUIRES MANUAL REVIEW`；构建会发出警告，并非获准分发。重新收集可运行 `go run scripts/release-audit/kubo-notices.go -output docs/kubo-dependency-licenses.txt`，需要网络；存在未决项时返回非零并保留证据文件。当前 Go 模块包含 go-ethereum，其库代码根据上游声明使用 LGPL-3.0；发布方必须进一步核实静态链接、对应源码和重链接等适用义务。不得将材料收集视为合规结论。

发布前还需核对 Kubo 所有依赖及第三方模板的许可、执行签名与验证、在干净的 Windows 系统中检查新装/跨版本升级/卸载与数据保留，并检验安装包实际内容。以上 Wine 测试不会覆盖这些事项。
