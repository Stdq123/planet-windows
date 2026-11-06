# 从发行源码重建与修改 Go 库

发行源码包包含指定 Git 提交的应用源码、`go.mod`、`go.sum`、`bun.lock`、Go vendor 源码和 go-ethereum 的源码及许可文本。`SOURCE-PROVENANCE.json` 记录提交、版本、锁文件摘要和 go-ethereum 模块版本及 h1；包外的 `source-manifest.json` 还记录 ZIP 的 SHA-256 和字节数。这些材料用于重建和修改静态链接的 Go 库，不是完整许可合规结论，也不能单独证明某个二进制由该源码构建。

源码包不包含 Git 历史、本机环境文件、发布者密钥、`node_modules`、已生成的前端或应用构建结果，以及 Kubo、WebView2、Go、Bun 等运行时或工具安装包。Go 模块按锁文件收集；Bun 依赖在重建时按 `bun.lock` 下载。go-ethereum 源码保留库、测试及构建说明，省略与链接库无关的上游可执行文件，具体路径列在清单的 `omittedUpstreamBinaries` 中。

## 准备发行源码包

在安装了 Go、Bun 和 Git 的 Windows 机器上，从干净的待发布提交运行：

```powershell
powershell -NoProfile -File scripts/prepare-source.ps1
```

脚本打印 `.mygo/source-releases/<版本>-<唯一编号>/` 下的源码 ZIP 和 `source-manifest.json` 路径；实际目录位于 `windows/`。它先导出已跟踪文件，在独立副本中执行 `go mod verify` 和 `go mod vendor`，补充 go-ethereum 的完整模块源码及 `COPYING`、`COPYING.LESSER`，然后再次验证依赖与锁文件。工作区有未提交改动、依赖缓存被修改、存在模块替换、源码包含本地凭据或构建输出、准备期间提交变化时都拒绝完成材料。

只导出当前提交，不读取被忽略的工作区内容。准备前仍应审查已跟踪源码，确保没有误提交的个人资料。脚本既不创建 Git 标签，也不推送或发布；公开发行须完成 [发行检查](releasing.md)，源码包与对应安装包一起提供。保留 `source-manifest.json` 和 `SHA256SUMS.txt`，下载后先核对摘要。

## 重建可运行的 Windows 程序

将源码 ZIP 解压到任何 Git 仓库之外的新的空目录，不要覆盖已有安装或真实 Planet 资料。在已有仓库的忽略目录内构建会让 Go 错误继承父仓库的版本信息；若必须这样构建，将下方 `GOFLAGS` 改为 `-mod=vendor -buildvcs=false`。使用 `windows/go.mod` 指定的 Go 工具链，以及 [Windows 指南](../windows/README.md#构建与测试) 中的 Bun 和 WebView2 环境。以下命令在解压后的 `windows/` 目录运行，执行前请记录已有环境变量，需要时在完成后恢复。

```powershell
bun install --frozen-lockfile
$env:GOWORK = 'off'
$env:GOFLAGS = '-mod=vendor'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
bun run build:web
# 如需本地 IPFS 节点，下载并校验固定版本的 Kubo：
powershell -NoProfile -File scripts/prepare-kubo.ps1
bunx mygo build -platform windows/amd64 -skip-build-command -skip-installer -o .mygo/rebuilt
```

程序输出为 `.mygo/rebuilt/windows-amd64/Planet Windows.exe`，包含重新构建的前端；`-skip-build-command` 避免再次执行面向发行包的材料收集，`-skip-installer` 仅生成独立程序和资源。该路径不需要发布者签名密钥，不会安装程序，也不会替换已有安装。它不能代替正式安装包的签名、许可或原生安装验收；需要对外分发修改版时，另行完成这些检查并清楚标识修改。

`-mod=vendor` 使 Go 使用包内 `windows/vendor/` 源码。不要在修改后运行 `go mod vendor`，该命令会从模块缓存重建 vendor 并覆盖修改。增加新的依赖或包时，应先更新依赖及 vendor，再进行所需修改。Go vendoring 的范围和行为见 [Go Modules Reference](https://go.dev/ref/mod#go-mod-vendor)。

## 修改 go-ethereum 并重新链接

go-ethereum 位于 `windows/vendor/github.com/ethereum/go-ethereum/`，库许可见其中的 `COPYING.LESSER`，同时保留 `COPYING`。在这个副本里修改需要调整的库实现，然后重复上述 `bunx mygo build` 命令即可将修改后的库静态链接进应用；无需改动原仓库或 Go 的全局模块缓存。

验证重链接时，可在独立实验副本中改变 `common/hexutil/hexutil.go` 的 `ErrEmptyString` 错误文本，加上唯一实验标记。该改动只用于证明不同库实现可被编译进应用，不作为正式产品修复。记录修改前后该源文件与生成程序的 SHA-256，验证新程序包含标记，并用临时用户资料目录运行 `--planet-version`。保留改动 diff、构建日志和实际输出；仅改变注释或看到构建成功不足以证明修改后的库行为已经进入二进制。

`--planet-version` 是只读命令。测试完整界面及资料操作时应使用隔离的新 Windows 用户或虚拟机，保留原安装与备份。源码包提供可修改和重建的工程材料；许可适用范围、分发义务及最终发布材料仍需单独审查。
