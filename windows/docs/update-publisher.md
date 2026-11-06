# 离线更新清单发布

`cmd/update-manifest` 在本地生成 Ed25519 密钥并签署更新清单。默认没有发布者更新源或受信任公钥；使用者需要明确配置并信任发布者公钥及 HTTPS 更新源，才能验证和获取更新。仅运行此工具不会配置客户端或部署服务。

在 Windows 的项目目录中构建发布者工具：

```powershell
go build -o update-manifest.exe ./cmd/update-manifest
```

## 生成密钥

在受控的离线环境执行，指定一个尚不存在的私钥文件：

```powershell
.\update-manifest.exe keygen -out D:\offline-secrets\planet-update.key
```

命令只输出 base64 编码的 32 字节公钥；请安全记录并通过可信渠道提供给使用者。文件中是 base64 编码的 64 字节 Ed25519 私钥。私钥不可作为命令行参数传入，不应提交到仓库、包含在安装包、发送给用户或上传到服务器。请离线备份并限制访问。Windows 创建新私钥时直接设置仅当前用户可访问的受保护 DACL，阻止继承父目录权限；其他系统以 `0600` 模式创建。`sign` 读取已有私钥时不会更改其权限，已有文件及备份的访问控制仍需自行维护。若文件已存在，命令拒绝覆盖。

## 签署和部署

先准备通过发行检查的最终安装包，然后在离线环境生成新的清单文件。下面以未来从 `0.0.1-alpha.1` 升级到 `0.0.1-alpha.2` 为例，不表示这两个版本已经发布：

```powershell
.\update-manifest.exe sign `
  -private-key D:\offline-secrets\planet-update.key `
  -installer D:\release\Planet-Setup.exe `
  -url https://downloads.example.com/Planet-Setup.exe `
  -version 0.0.1-alpha.2 `
  -minimum-version 0.0.1-alpha.1 `
  -valid-days 7 `
  -out D:\release\update.json
```

`-minimum-version` 默认为 `0.2.0`，为 `0.0.1` 系列生成清单时必须显式指定经过验证的最低版本。`-valid-days` 默认为 7，可设置 1–30 天。版本主体必须是三个无前导零的非负整数，每段不超过 4294967295，可带 SemVer 预发布后缀；纯数字预发布标识不能有前导零，不接受 `+` 构建元数据。最低版本不能高于发布版本。安装包必须非空且不大于 512 MiB。URL 必须使用 HTTPS，路径以 `.exe` 结尾，不能包含用户凭据或片段。`-out` 必须指向尚不存在的文件。

更新器与安装器使用相同的版本排序：`0.0.1-alpha.2 < 0.0.1-alpha.10 < 0.0.1`。预发布版可以获取更高版本的预发布或稳定版；已安装稳定版的更新检查跳过预发布。当前 `0.0.1-alpha.1` 目标低于早期 `0.2.0`/`0.3.0` 开发快照，不能通过清单把它作为这些安装的升级推送。开发快照迁移仍需另行验证，见 [升级与恢复](safe-upgrade.md)。

工具流式计算安装包 SHA-256 和大小，并对原始 JSON 载荷签名；`update.json` 包含 base64 编码的载荷和签名。将**完全相同的安装包**及清单通过可信的部署流程上传至 HTTPS 服务。客户端要求清单地址和安装包地址同源，包括主机和端口；工具本身不知道客户端配置的清单地址，部署前需要检查。有效期从签署时间开始计算，过期后须生成并部署新的清单。

该工具不生成或验证安装包的 Authenticode 签名。更新清单签名用于验证清单和指定的安装包摘要，Windows 安装包的发布者身份与系统信任仍需单独配置和验证。alpha 同样需要完成 [发行流程](../../docs/releasing.md) 中的签名、许可和原生 Windows 验收；生成清单不代表获准分发。
