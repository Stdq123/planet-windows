# Planet Windows

A work-in-progress Windows application for publishing websites with IPFS and following web feeds. Planet Windows is an independent Windows port inspired by and derived from [Planet](https://github.com/Planetable/Planet). It uses Go, MyGo, WebView2 and TypeScript.

**Status:** early development. This project does not yet implement all Planet features, and compatibility with real macOS backups has not been verified. Keep the original backups and test recovery before relying on them. See the [detailed Chinese guide](windows/README.md) for supported workflows and limitations.

## What works

- Create local sites and Markdown posts, preview them, and export basic static HTML.
- Publish supported sites to IPFS and IPNS with a bundled Kubo node. Saving an article triggers background publication of the saved site and attachments.
- Follow and manually refresh RSS, Atom and JSON feeds.
- Import selected Planet backups and export backups containing IPNS keys. Some imported sites can be regenerated using verified versions of the Plain, 8-bit and Grid templates; support varies by site and template.

Published content is public. Long-term availability depends on other nodes or pinning services. Exported backups contain unencrypted IPNS private keys; store them securely. For supported import scenarios, restrictions and known gaps, read [windows/README.md](windows/README.md).

## Build and test

Windows x64, WebView2 Runtime, Bun and Go 1.27.3 or a compatible toolchain are required. With `GOTOOLCHAIN=auto`, Go can download the required toolchain.

```powershell
cd windows
bun install --frozen-lockfile
bun test
go test ./...
bun run typecheck
bun run build
```

The build downloads Kubo 0.38.2 from its official release and verifies its SHA-512 checksum before packaging it. Output is placed under `windows/build/windows-amd64/`. Default tests do not publish to the IPFS network; optional Kubo, online and template tests are documented in [windows/README.md](windows/README.md).

## Contributing and support

See [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). For bugs and feature requests, use the [GitHub issue tracker](https://github.com/puremixai/planet-windows/issues). For security concerns, see [SECURITY.md](SECURITY.md).

The source snapshot is also available on [Gitea](https://gitea.com/puremixai/planet-windows) and [Codeberg](https://codeberg.org/puremixai/planet-windows). GitHub is the preferred place for collaboration. These are independent repositories; synchronization is manual.

## Origin and license

This repository contains a snapshot of the Windows port from the Planet-derived working tree, rather than the original full upstream Git history. Original Planet copyright notices are retained in [LICENSE](LICENSE). Planet Windows is distributed under the MIT license; third-party dependencies and bundled components have their own licenses. See [NOTICE.md](NOTICE.md).
