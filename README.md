# Planet Windows

A work-in-progress Windows application for publishing websites with IPFS and following web feeds. Planet Windows is an independent Windows port inspired by and derived from [Planet](https://github.com/Planetable/Planet). It uses Go, MyGo, WebView2 and TypeScript.

**Status:** early development. This project does not yet implement all Planet features, and compatibility with real macOS backups has not been verified. Keep the original backups and test recovery before relying on them. See the [detailed Chinese guide](windows/README.md) for supported workflows and limitations.

## Screenshot

![Planet Windows showing the subscription list, article list and an article with inline images](docs/screenshots/subscription-reader.png)

Reading olivida.eth in Planet Windows.

## Implemented capabilities

The following features are available in the development version; see the limitations and validation status below before relying on them.

- Create local sites, edit Markdown posts and private drafts, manage attachments, and export static HTML.
- Publish supported sites to IPFS and IPNS with a bundled Kubo node. Saving an article triggers background publication of the saved site and attachments.
- Follow and refresh RSS, Atom, JSON Feed, IPNS and supported ENS sites. Discover feeds from supported website homepages, read inline media, and keep selected content available offline.
- Import selected Planet backups and export backups containing IPNS keys. Compatible sites can be rebuilt using verified versions of Plain, 8-bit, Grid, Platinum and Sepia.
- Configure Filebase or Pinnable for remote pinning, and Plausible for site analytics.
- Check for signed updates through a configured HTTPS feed, download a verified installer, and run it to upgrade a compatible installation.

Published content is public. Long-term availability depends on other nodes or pinning services. Exported backups contain unencrypted IPNS private keys; store them securely. For supported import scenarios, restrictions and known gaps, read [windows/README.md](windows/README.md).

## Incomplete and awaiting validation

- **Mac compatibility:** Real Mac backups and round trips have not been verified. Some imported-site settings cannot be edited, and unsupported backup contents can prevent rebuilding or export. Keep the original backup.
- **Templates and integrations:** Only the listed template versions and supported settings are implemented. Other original Planet features and integrations remain incomplete.
- **External services:** The Filebase, Pinnable and Plausible integrations and SSH/Cloudflare deployments have been tested with local simulations; they have not been verified with real accounts or servers.
- **ENS and media:** Non-IPFS/IPNS ENS content is unsupported. Live end-to-end testing of ENS/IPNS content with a subpath remains outstanding; media format support depends on available Windows components.
- **Distribution:** Native Windows installer and uninstaller end-to-end validation, executable signing, a preconfigured update source and parts of the third-party license review remain outstanding. See the [release audit](windows/docs/release-audit.md).

## Follow a site

1. Select **+** next to **订阅** (Subscriptions) in the sidebar.
2. Enter a feed URL, a supported website URL such as `https://olivida.eth.limo`, an IPNS address, or an ENS name such as `olivida.eth`, then select **添加** (Add).
3. Select the subscription and an article to read it. Use **刷新订阅** (Refresh subscription) to check for updates, or the star in the article view to save a favorite.
4. To keep supported content offline, select **更多操作 → 固定离线** (More actions → Pin offline) while viewing a subscription.

Website URLs are followed as web feeds when their pages declare a supported feed; a bare `.eth` name uses the ENS/IPFS subscription path. If a website does not declare a feed, enter its feed URL directly. Online web-feed articles can display images and inline audio or video. Pinned subscriptions use locally saved media for offline reading; availability and size limits depend on the subscription type. See the [Windows guide](windows/README.md) for details.

## Build and test

Windows x64, WebView2 Runtime, Bun and Go 1.27.3 or a compatible toolchain are required. The installer build also needs 7-Zip. With `GOTOOLCHAIN=auto`, Go can download the required toolchain.

```powershell
cd windows
bun install --frozen-lockfile
bun run check
bun run build
```

The build downloads Kubo 0.38.2 from its official release and verifies its SHA-512 checksum before packaging it. The build prints the release directory under `windows/.mygo/releases/<build-id>/windows-amd64/`. Default tests run without publishing to IPFS. See the optional Kubo, online and template tests in the [Windows guide](windows/README.md).

`bun run check` is the shared local and CI entry point for Go formatting, static analysis, frontend and Go tests, TypeScript checks and the frontend production build. It stops on the first failure. Installer validation remains part of `bun run build` and the release audits.

Windows CI runs the shared checks and dependency security scan on pull requests and pushes to `main`. Use `bun run security:check` for local online dependency advisory checks. The [maintenance standard](docs/maintenance.md) records repository protection and validation requirements; the [release guide](docs/releasing.md) explains Changelog maintenance, local draft materials, version checks and native Windows acceptance.

## Contributing and support

See [CONTRIBUTING.md](CONTRIBUTING.md), the [Git workflow guide (Chinese)](docs/git-workflow.md), [CHANGELOG.md](CHANGELOG.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). For bugs and feature requests, use the [GitHub issue tracker](https://github.com/Stdq123/planet-windows/issues). For security concerns, see [SECURITY.md](SECURITY.md).

The source snapshot is also available on [Gitea](https://gitea.com/puremixai/planet-windows) and [Codeberg](https://codeberg.org/puremixai/planet-windows). GitHub is the preferred place for collaboration. These are independent repositories; synchronization is manual.

## Origin and license

This repository is the development home of the Windows port. Its history begins with a snapshot of the Planet-derived working tree. Original Planet copyright notices are retained in [LICENSE](LICENSE). Planet Windows is distributed under the MIT license. See the separate licenses for third-party dependencies and bundled components in [NOTICE.md](NOTICE.md).
