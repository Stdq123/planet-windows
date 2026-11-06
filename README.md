# Planet Windows

> **Upstream project: [Planetable/Planet](https://github.com/Planetable/Planet)**
>
> Planet Windows is an independent Windows port derived from this upstream project.

Publish websites with IPFS and follow web feeds from a Windows desktop app, built with Go, MyGo, WebView2 and TypeScript.

**Early development:** not all original Planet features are implemented, and compatibility with real macOS backups has not been verified. Keep original backups and test recovery before relying on imported data.

**First public prerelease: `v0.0.1-alpha.1`.** [Downloads and release notes](https://github.com/Stdq123/planet-windows/releases) include the Windows x64 installer, corresponding source and SHA-256 checksums. This alpha is **unsigned** and has a deliberately limited [acceptance scope](docs/acceptance-0.0.1-alpha.1.md). Earlier `0.3.0` and other version numbers identify untagged development snapshots; existing `0.2.0`/`0.3.0` installations reject this version as a downgrade. Test alpha in an isolated, fresh Windows user profile and preserve the original installation and data.

[中文使用指南](windows/README.md) · [Build and test](#build-and-test) · [Known limitations](#incomplete-and-awaiting-validation) · [Contributing](CONTRIBUTING.md) · [Changelog](CHANGELOG.md)

## Screenshot

![Planet Windows showing the subscription list, article list and an article with inline images](docs/screenshots/subscription-reader.png)

Reading olivida.eth in Planet Windows.

## Implemented capabilities

These capabilities are available in the development version, within the support and validation limits described in the [Windows guide](windows/README.md).

| Area | What you can do |
| --- | --- |
| Writing | Manage local sites, Markdown posts, private drafts, tags and attachments; resume work from the all-sites writing dashboard. |
| Publishing | Publish supported sites through a bundled Kubo node to IPFS and IPNS, or export basic static HTML. Local sites also support SSH and Cloudflare Pages deployment. |
| Reading | Follow RSS, Atom, JSON Feed, IPNS and supported ENS sites; discover feeds from supported website homepages, read inline media, search cached content and pin supported content for offline reading. |
| Backups and templates | Import selected Planet backups, export backups with IPNS keys, and rebuild compatible sites with verified versions of Plain, 8-bit, Grid, Platinum and Sepia. |
| Site services | Configure Filebase or Pinnable for remote pinning and Plausible for traffic analytics. |
| Article sync | Send article titles, summaries and links to one Telegram group or channel per site. For X, optionally sync new articles after IPNS publication or manually send existing published articles. See the [Telegram](windows/README.md#telegram-文章同步) and [X](windows/README.md#同步文章到-xtwitter) setup guides. |
| Updates | Check a configured HTTPS feed for signed update metadata, download a verified installer and run it to upgrade a compatible installation. |

Saving an article triggers background publication of the entire saved site and its attachments. Private drafts remain local until published. Published content is public; long-term availability depends on other nodes or pinning services. Exported backups contain **unencrypted IPNS private keys** and must be stored securely.

## Follow a site

1. Select **+** next to **订阅** (Subscriptions) in the sidebar.
2. Enter a feed URL, a supported website URL such as `https://olivida.eth.limo`, an IPNS address, or an ENS name such as `olivida.eth`, then select **添加** (Add).
3. Select the subscription and an article to read it. Use **刷新订阅** (Refresh subscription) to check for updates, or the star in the article view to save a favorite.
4. For offline reading, select **更多操作 → 固定离线** (More actions → Pin offline) while viewing a subscription.

Website URLs are followed as web feeds when their pages declare a supported feed. If a website does not declare one, enter its feed URL directly. A bare `.eth` name uses the ENS/IPFS subscription path.

Online web-feed articles can display images and inline audio or video. Pinned subscriptions use locally saved media; supported formats and size limits depend on the subscription type. See the [Windows guide](windows/README.md) for writing, publishing, offline storage and service configuration.

## Build and test

### Requirements

- Windows x64 with WebView2 Runtime.
- Git, Bun and Go 1.27.3 or a compatible toolchain. With `GOTOOLCHAIN=auto`, Go can download the required toolchain.
- 7-Zip for the installer build.

Commands below use PowerShell. Start in the repository root:

```powershell
cd windows
bun install --frozen-lockfile
```

### Run the desktop app in development

Prepare Kubo for IPFS features, then start MyGo:

```powershell
powershell -NoProfile -File scripts/prepare-kubo.ps1
bun run dev
```

MyGo starts the Vite development server and opens the desktop app. `bun run dev:web` starts only the frontend server; desktop functionality requires the MyGo runtime.

### Check and package

```powershell
bun run check
bun run build
```

`bun run check` is the full local and CI entry point. It runs documentation checks, Go formatting checks, `go vet`, frontend tests, uncached Go tests, TypeScript checks and the frontend production build, stopping on the first failure. Default tests do not publish to IPFS; optional Kubo, online and template tests are documented in the [Windows guide](windows/README.md#构建与测试).

For changes limited to the [documentation allowlist](docs/git-workflow.md#纯文档检查), run `bun run check:docs` with Git and Bun only. It checks text formatting and local links without installing dependencies or building the app. CI and the optional pre-push hook select this path automatically; code, mixed changes and uncertain comparisons retain full checks.

Close running Planet and Kubo processes before `bun run build`. The build prepares Kubo 0.38.2 from its official release, verifies the archive's SHA-512 checksum, and packages the app and installer. It prints both output paths:

| Output | Location, relative to the repository root |
| --- | --- |
| Latest app, Kubo and installer | `windows/build/windows-amd64/` |
| Separate copy for each build | `windows/.mygo/releases/<build-id>/windows-amd64/` |
| Previous latest build, when replaced | `windows/build/.previous-<ID>/` |

A successful quality check does not validate the installer. Packaging and native Windows acceptance remain separate steps; see the [release audit](windows/docs/release-audit.md) and [release guide](docs/releasing.md).

### Other commands

Run these from `windows/`:

| Command | Purpose |
| --- | --- |
| `bun run generate` | Regenerate the TypeScript bindings for Go methods. |
| `bun run test` | Run frontend tests. |
| `bun run check:docs` | Check documentation encoding, Markdown fences and local links without a build. |
| `go test ./...` | Run Go tests; optional integration tests require explicit setup. |
| `bun run typecheck` | Check TypeScript types. |
| `bun run build:web` | Check TypeScript and build frontend assets. |
| `bun run security:check` | Query online dependency advisories; required for dependency changes and release preparation. |
| `bun run release:check --tag vX.Y.Z` | Validate the supplied release tag against the app version and dated Changelog entry; use the full intended tag, including any prerelease suffix. |
| `bun run release:prepare -BundleDir <bundle-directory>` | Prepare local draft release materials; see the release guide for required inputs and acceptance. |

Windows CI runs the required `Windows quality checks` job on pull requests and pushes to `main`: documentation-only changes use the lightweight check, while other changes run the full checks and dependency security scan. Manual runs always use full checks. Repository protection and validation requirements are recorded in the [maintenance standard](docs/maintenance.md).

## Incomplete and awaiting validation

- **Mac compatibility:** Real Mac backups and round trips have not been verified. Some imported-site settings cannot be edited, and unsupported backup contents can prevent rebuilding or export. Keep the original backup.
- **Templates and integrations:** Only the listed template versions and supported settings are implemented. Other original Planet features and integrations remain incomplete.
- **External services:** Telegram, X, Filebase, Pinnable and Plausible integrations and SSH/Cloudflare deployments have been tested with local simulations; they have not been verified with real accounts or servers.
- **ENS and media:** Non-IPFS/IPNS ENS content is unsupported. Live end-to-end testing of ENS/IPNS content with a subpath remains outstanding; media format support depends on available Windows components.
- **Distribution:** Native silent installer and uninstaller checks passed on Windows Server 2025. Full Windows 10/11 desktop, input-method and recovery acceptance remain incomplete. This alpha is unsigned and has no preconfigured update feed; download it manually. Signed update metadata does not mean the executable or installer has a code signature. See the [acceptance scope](docs/acceptance-0.0.1-alpha.1.md) and [release audit](windows/docs/release-audit.md).

## Project layout

```text
windows/
  main.go           Desktop entry point and Go methods exposed to the UI
  src/              TypeScript frontend and styles
  internal/         Storage, publishing, feeds, templates and integrations
  tests/            Frontend and release-tool tests
  scripts/          Kubo preparation, packaging and installer checks
  docs/             Upgrade, update-publishing and release-audit guides
  release.json      Shared application version
scripts/            Repository quality, security and release checks
docs/               Collaboration, maintenance and release documentation
```

## Documentation

| Resource | Contents |
| --- | --- |
| [中文使用指南](windows/README.md) | Supported workflows, service setup, compatibility limits and optional tests. |
| [Contributing](CONTRIBUTING.md) and [Git workflow (中文)](docs/git-workflow.md) | Development conventions, branches, commits and pull requests. |
| [Release guide](docs/releasing.md) | Version checks, draft materials and native Windows acceptance. |
| [Upgrade and recovery (中文)](windows/docs/safe-upgrade.md) | Installer compatibility and recovery procedures. |
| [Update publisher guide (中文)](windows/docs/update-publisher.md) | Signed update manifests and publisher tooling. |
| [Changelog](CHANGELOG.md) | User-visible changes. |
| [Security policy](SECURITY.md) and [Code of conduct](CODE_OF_CONDUCT.md) | Security reporting and community guidelines. |

## Contributing and support

Use the [GitHub issue tracker](https://github.com/Stdq123/planet-windows/issues) for bugs and feature requests, and follow [CONTRIBUTING.md](CONTRIBUTING.md) when submitting changes. For security concerns, follow [SECURITY.md](SECURITY.md).

Source snapshots are also available on [Gitea](https://gitea.com/puremixai/planet-windows) and [Codeberg](https://codeberg.org/puremixai/planet-windows). GitHub is the preferred place for collaboration. These are independent repositories; synchronization is manual.

## Origin and license

This repository is the development home of the Windows port. Its upstream project is [Planetable/Planet](https://github.com/Planetable/Planet), and its history begins with a snapshot of the Planet-derived working tree. Original Planet copyright notices are retained in [LICENSE](LICENSE). Planet Windows is distributed under the MIT license; separate licenses for third-party dependencies and bundled components are listed in [NOTICE.md](NOTICE.md).
