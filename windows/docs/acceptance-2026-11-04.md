# 2026-11-04 验证记录

交付候选：`.mygo/releases/7c450b4aa907330b40132955b04f5fbf/windows-amd64/planet-windows-safe-setup-0.2.0-amd64.exe`。

安装包 SHA-256：`8768136b9ae70604a3cc1f6b7592595f966d23d78a017857c36d9fab56035f83`。10 项安装 payload 与旁边的独立文件逐项哈希一致。Authenticode 未签名；Kubo 依赖仍有 4 项许可需人工核对。

- 设置原版模板及 Kubo 路径后的 `go test -count=1 ./...` 通过；最后的安装器版本状态修复后，安装器守卫完整测试与 `go vet` 再次通过。
- `bun test`：54 pass，0 fail；`bun run typecheck`、`go vet ./...`、`git diff --check` 通过。
- 原生 Windows 程序在独立临时 APPDATA/LOCALAPPDATA 中启动，实际持有运行期互斥对象；格式 3 的构造资料库成功迁移为格式 4，原始字节快照一致。
- 原版备份默认 `prewarmNewPost: true` 的离线 Kubo 导入、改文、删文、导出、重导入回归通过；验证私钥匹配、附件及未知备份文件保留、公开输出不含私有配置。
- 日期过滤器跨年及不同时区测试、原版 Plain/Paper 页面渲染测试通过。没有开放 Paper 的完整安装兼容。
- 最终安装包在禁网 Docker/Wine 中通过 0.2 → 测试 0.3、运行中锁拒绝、同包重复、降级拒绝、卸载、未知文件/用户数据/快捷方式保留及旧安全 0.1/MyGo 安装拒绝。

隔离验收命令：

```powershell
powershell -NoProfile -File scripts/release-audit/test-transaction-upgrade.ps1 `
  -OldInstaller '.mygo/releases/7c450b4aa907330b40132955b04f5fbf/windows-amd64/planet-windows-safe-setup-0.2.0-amd64.exe' `
  -NewInstaller '.mygo/release-audit-tests/upgrade-fixture-693b036c9cfe469ca27082d3833d8c46/payload/TEST-ONLY-upgrade-setup.exe' `
  -LegacySafeInstaller '.mygo/releases/197b1806520b401e9738fce6b481100c/windows-amd64/planet-windows-safe-setup-0.1.0-amd64.exe' `
  -LegacyInstaller 'build/phase11/windows-amd64/planet-windows-setup-0.1.0-amd64.exe'
```

测试 0.3 的 exe 通过附加 PE overlay 标记产生不同字节，验证真实文件替换；它不包含不同功能逻辑，不能当作真实发行版本间的数据兼容性验收，也不能交付。原生单元测试覆盖各文件切换检查点的中断与重试，集成测试未执行真实父子进程强杀或断电。未进行原生 Windows 虚拟机的完整安装生命周期验收，也没有真实 Mac 备份可用于往返验证。测试未修改或发布用户的 test 站点。
