# Planet Windows（开发中）

这是 Planet 的独立 Windows 移植，使用 Go、MyGo、WebView2 和 TypeScript，不使用 Electron 或 Swift。当前为开发版本，尚不具备原版的完整功能；原版数据兼容性仍需要真实 Mac 备份验证。

## 当前能力

- 创建多个本地站点，创建、编辑、保存和删除 Markdown 文章，预览文章。
- 保存文章后自动在后台公开发布整个站点及其已保存附件；连续保存会在正在进行的发布完成后再次发布最新保存的内容。未完成的发布会在下次启动时重试。发布失败时文章仍保存在本地，可用“发布到 IPFS”重试。
- Windows 创建的站点可为已保存的文章添加或删除本地附件；附件立即保存或删除，系统插入的 Markdown 引用需要保存文章，删除附件时须自行删除不再需要的引用。发布、网页导出及备份会携带已保存附件。发布期间的附件操作可能需要等待发布结束，等待时不能保存或切换站点。编辑器的隔离预览暂不显示本地图片，发布后的页面可引用图片。
- 将已保存的站点导出为基础静态 HTML 网站。该输出使用简化模板，不等同于原版 Planet 网站及模板格式。
- 添加、读取和手动刷新 RSS、Atom、JSON Feed；已获取的订阅内容缓存在本机。预览为安全隔离，外部图片及脚本不会加载。
- 新建的本地站点可以使用随安装包提供的 Kubo 发布到 IPFS 和 IPNS；发布成功后可在站点界面复制完整的网关访问地址。应用运行时提供已发布内容，并定期尝试续期 IPNS。节点离线后内容的持续可用性取决于其他节点或固定服务，公网网关可访问性也取决于网络环境。
- 可以选择原版 `.planet` 目录导入，并导出包含私钥的备份。导入会核对 IPNS 密钥并保留原版附件和未知元数据；测试使用构造的备份样本，尚未在 Mac 版做双向验证。
- 简单 Plain 导入站点在选择已验证的 Plain 模板后，可从文章数据重新生成网页、复制原版登记的附件并使用原 IPNS 身份发布。不会上传原始备份元数据、私钥或未登记文件；不符合检查的备份将被拒绝。
- 可以从本地选择原版 Plain build 132、8-bit build 50 和 Grid build 99；文本站点的首页、文章页、归档页和分页已用模板样本验证。其他模板尚不支持。

选择模板时应选择含 `template.json`、`templates` 和 `assets` 的目录。当前验证的修订分别为 [Plain](https://github.com/Planetable/SiteTemplatePlain) `b29140fd906877206a76edb5e9510f816d16e336`、[8-bit](https://github.com/Planetable/SiteTemplate8bit) `88ac27ae3d9a34787690f45efa6e7516b910f562` 和 [Grid](https://github.com/Planetable/SiteTemplateGrid) `10b5bccda25a1028a22b62b88ea6ccfbbfcfcd52`；程序校验模板文件内容，执行模板中的 `hooks` 不受支持。

应用的数据使用独立的 MyGo 用户数据目录和 `library.json`，导入时不会修改原备份。备份包含未加密的 IPNS 私钥，请自行妥善保管。

## 尚未完成

导入站点暂不能在 Windows 中添加或删除附件。普通 Plain 站点可以选择已验证的 Plain 模板，并在编辑已有文章后重建网页、发布或导出备份；涉及不支持的站点设置、媒体、slug、自定义路径、额外文章元数据、增删文章或特殊生成文件时会拒绝重建，以免内容遗漏。Windows 附件暂不支持音视频元数据处理。暂不支持 ENS/Planet 订阅、固定服务、音频播放或其他原版集成。实际 Mac 备份和其他模板尚未完成兼容性验证；请保留原始备份。

## 构建与测试

需要 Windows 64 位、Go 1.27.3 或兼容工具链、Bun 和 WebView2 Runtime。Go 使用默认的 `GOTOOLCHAIN=auto` 时可以自动获取依赖所要求的工具链。

```powershell
cd windows
bun install --frozen-lockfile
bun test
go test ./...
bun run build
```

构建产物位于 `build/windows-amd64/`，包括程序和安装包。`go test ./...` 默认跳过真实 Kubo 集成测试；如已有可信的 Kubo Windows 程序，可设置 `PLANET_TEST_KUBO` 指向该程序，再运行 `go test -count=1 ./internal/ipfs`。测试只使用临时仓库和测试密钥执行离线操作。

构建脚本从 Kubo 官方 GitHub release 下载固定版本 0.38.2 并核验 SHA-512，构建时将程序及许可证放入安装包。默认测试不会访问 IPFS 公网；明确设置 `PLANET_TEST_ONLINE=1` 时才会发布公开的合成测试内容，使用第二个临时节点验证解析。模板测试需要将 `PLANET_TEST_TEMPLATE_ROOT` 指向本地 Plain 模板目录、`PLANET_TEST_TEMPLATE` 指向其 `templates` 子目录，`PLANET_TEST_TEMPLATES_DIR` 指向含 `plain`、`gamedb`、`grid` 子目录的原版模板目录。

上游项目以 MIT 许可证发布；本目录使用的第三方依赖及其许可证应随最终发行包核验和提供。
