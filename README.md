# Planet Windows

A work-in-progress Windows application for publishing websites with IPFS and following web feeds. Planet Windows is an independent Windows port inspired by and derived from [Planet](https://github.com/Planetable/Planet). It uses Go, MyGo, WebView2 and TypeScript.

**Status:** early development. This project does not yet implement all Planet features, and compatibility with real macOS backups has not been verified. Keep the original backups and test recovery before relying on them. See the [detailed Chinese guide](windows/README.md) for supported workflows and limitations.

## Screenshot

![Planet Windows showing the subscription list, article list and an article with inline images](docs/screenshots/subscription-reader.png)

Reading olivida.eth in Planet Windows.

## What works

- Create local sites, edit Markdown posts and private drafts, manage attachments, and export static HTML.
- Publish supported sites to IPFS and IPNS with a bundled Kubo node. Saving an article triggers background publication of the saved site and attachments.
- Follow and refresh RSS, Atom, JSON Feed, IPNS and supported ENS sites, and keep selected content available offline.
- Import selected Planet backups and export backups containing IPNS keys. Compatible sites can be rebuilt using verified versions of Plain, 8-bit, Grid, Platinum and Sepia.
- Configure Filebase or Pinnable for remote pinning, and Plausible for site analytics.
- Check for signed updates through a configured HTTPS feed, download a verified installer, and run it to upgrade a compatible installation.

Published content is public. Long-term availability depends on other nodes or pinning services. Exported backups contain unencrypted IPNS private keys; store them securely. For supported import scenarios, restrictions and known gaps, read [windows/README.md](windows/README.md).

## Build and test

Windows x64, WebView2 Runtime, Bun and Go 1.27.3 or a compatible toolchain are required. The installer build also needs 7-Zip. With `GOTOOLCHAIN=auto`, Go can download the required toolchain.

```powershell
cd windows
bun install --frozen-lockfile
bun test
go test ./...
bun run typecheck
bun run build
```

The build downloads Kubo 0.38.2 from its official release and verifies its SHA-512 checksum before packaging it. The build prints the release directory under `windows/.mygo/releases/<build-id>/windows-amd64/`. Default tests run without publishing to IPFS. See the optional Kubo, online and template tests in the [Windows guide](windows/README.md).

## Contributing and support

See [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). For bugs and feature requests, use the [GitHub issue tracker](https://github.com/Stdq123/planet-windows/issues). For security concerns, see [SECURITY.md](SECURITY.md).

The source snapshot is also available on [Gitea](https://gitea.com/puremixai/planet-windows) and [Codeberg](https://codeberg.org/puremixai/planet-windows). GitHub is the preferred place for collaboration. These are independent repositories; synchronization is manual.

## Origin and license

This repository is the development home of the Windows port. Its history begins with a snapshot of the Planet-derived working tree. Original Planet copyright notices are retained in [LICENSE](LICENSE). Planet Windows is distributed under the MIT license. See the separate licenses for third-party dependencies and bundled components in [NOTICE.md](NOTICE.md).
