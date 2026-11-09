# Changelog

User-facing changes are recorded here. Release entries use `MAJOR.MINOR.PATCH` with an optional SemVer prerelease suffix and an immutable Git tag containing `v` plus the complete version. Build metadata is not accepted. Earlier development snapshots are described in the [Windows guide](windows/README.md); this file does not reconstruct unverified release dates.

## [Unreleased]

### Changed

- Modernized the light interface in place while keeping the graphite direction and layout: legacy blue-gray and Fluent-blue literals now read the shared semantic tokens, font sizes and radii follow unified scales with an 11px text floor, spacing rhythm and heading weights are consistent, and sidebar/overview transitions use shared duration and easing tokens. Functional colors (traffic chart, site icon tints, import warning) are unchanged.

### Fixed

- Improved the contrast of input, select, secondary-button and radio/checkbox-card boundaries on the Windows light surfaces.
- Improved editor placeholders, selected-row details, sidebar errors, IPFS chart labels, removed-site IDs and template-library text contrast.
- Unified error and destructive-action colors, added non-color selection indicators to navigation, article rows, the first draft and editor view controls, and aligned template selection with the graphite palette.
- Kept the last loaded dashboard visible when a background refresh fails, with an error message and an available refresh action to retry.
- Restored the saved public gateway in the visible radio group at startup so its selected option matches the links in use.
- Aligned the compact dashboard's visual and keyboard order: at widths up to 1100px, drafts and recent articles now precede site status and analytics; the site-status heading and create-site action remain visible.
- Marked rejected form fields with a danger-colored boundary in site, article and X sync settings, including converted radio-card choices, so the field behind an error message is visible without relying on focus alone.

## [0.0.1-alpha.2] - 2026-11-06

### Changed

- Unified the Windows application, installer and uninstaller icons, sidebar brand mark and web-preview favicon around the Neighbor Pages identity, with maintained SVG logo sources and a branding guide.
- Reorganized the README around downloading and getting started, with clearer Windows prerequisites and known limitations.

### Known limitations

- This remains an experimental, unsigned Windows x64 prerelease. SHA-256 checks confirm file integrity, not publisher identity. The final installer must pass the disposable Windows Server 2025 native smoke workflow before publication; the release includes its package-specific evidence.
- Windows 10/11 GUI, WebView2 failure handling, Chinese input, DPI transitions, real cross-version upgrades, interrupted upgrades and power-loss recovery remain unverified. Real Mac backup compatibility and live external-service integrations are also unverified. Use an isolated Windows user profile with disposable data and keep original backups; see the [acceptance scope](https://github.com/Stdq123/planet-windows/blob/v0.0.1-alpha.2/docs/acceptance-0.0.1-alpha.2.md).
- No production update feed is configured. Download manually; existing alpha.1 installations are not claimed to have passed cross-version upgrade acceptance. Earlier `0.2.0`/`0.3.0` development installations cannot downgrade to this version.

## [0.0.1-alpha.1] - 2026-11-06

### Known limitations

- This is an experimental, unsigned Windows x64 prerelease. The application and installer have no Authenticode publisher signature; SHA-256 checks confirm file integrity only.
- Native silent installation, same-package reinstallation, refusal of locked or modified files, and uninstall with data preservation have passed on Windows Server 2025. Full Windows 10/11 GUI, WebView2 failure handling, Chinese input, interrupted upgrades and power-loss recovery remain unverified. These are explicitly accepted alpha limitations, not passed checks; see the [acceptance scope](https://github.com/Stdq123/planet-windows/blob/main/docs/acceptance-0.0.1-alpha.1.md).
- Use a fresh Windows user profile for alpha testing. Earlier `0.2.0`/`0.3.0` development installations cannot downgrade to this version. Real Mac backup round trips and live external-service integrations remain unverified; keep original backups.
- No production update feed is preconfigured. Download this alpha manually from GitHub Releases. Corresponding source, rebuild instructions, SHA-256 checksums and the final package audit accompany the release.

### Security

- Updated Markdown rendering to reject encoded and automatic script URLs ([GO-2026-5320](https://pkg.go.dev/vuln/GO-2026-5320)), with a regression test covering links and images.
- Updated the HTML parser and renderer to fix reachable issues in untrusted feed and preview content ([GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860), [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866)).

### Added

- Corresponding source archives containing pinned Go dependencies and go-ethereum source, with release-time commit and checksum validation and library rebuilding instructions.
- A manually triggered installer smoke workflow that uses a disposable native Windows VM and retains the tested package with installation, refusal and uninstall evidence.
- Optional X (Twitter) article sync after successful IPNS publication, with encrypted local credentials, manual history sync, persistent delivery status and explicit recovery for uncertain requests.
- Per-site Telegram group or channel synchronization after successful publication, with title, summary and article link, encrypted local bot credentials, duplicate suppression, message updates and manual retry.
- An all-sites startup overview with local content totals, recent drafts and articles, publication states, and on-demand IPFS and Plausible access.
- A single `bun run check` command for Go formatting and static analysis, frontend and Go tests, TypeScript checks and the frontend production build.
- Optional Git hooks to catch staged whitespace errors before commits and run the shared checks before pushes.
- A Chinese Git workflow guide covering branches, focused commits, pull requests, versioning and mirror synchronization.
- Weekly dependency update proposals for GitHub Actions, Go modules and Bun packages, with up to three open version-update PRs per ecosystem, plus security-update PRs for supported dependencies. Updates require local verification, passing CI and manual merging.
- Local release preparation with version/Changelog checks, explicit draft notes, installer SHA-256, audit results and a source-checkout record.
- Local Bun and Go vulnerability checks, plus documented native Windows release acceptance and recovery requirements.
- Main-branch and release-tag protection rules, maintainer routing and private security-report guidance.

### Changed

- Retain all five bundled templates and their pinned upstream sources, attribution and available license texts, and remove the project's template pending-review release gate. This does not assign new licenses to upstream assets.
- Complete four missing Kubo notice entries using pinned source and nested-license evidence, with exact module and file checks that reject changed or unsupported inputs.
- Set the first public prerelease to `0.0.1-alpha.1`, resetting the untagged development numbering. Release preparation, update manifests and the installer support SemVer prereleases with numeric identifier ordering. Stable installations skip prerelease updates; prerelease installations can update to later prereleases or stable versions. Existing `0.2.0`/`0.3.0` development installations reject this version as a downgrade; use an isolated, fresh Windows user profile for alpha testing.
- Allow explicitly selected unsigned prereleases through `release:prepare -Release -AllowUnsignedPrerelease`, with actual signing status, SHA-256 checksums and an unsigned distribution notice. Stable releases still require valid Authenticode signatures; invalid signatures remain blocked. Checksums do not authenticate publisher identity, and the exception does not waive license review or native Windows acceptance.
- Documentation-only pushes and pull requests now run lightweight encoding, Markdown and local-link checks without application tests or a frontend build; code, mixed changes and manual CI runs retain the full quality checks.
- Renamed the Chinese overview navigation, heading, refresh label and loading/error messages to “写作台”.
- Rebuilt the all-sites overview from the approved Figma writing workspace, with the subsequent neutral-palette revision: warm white surfaces, graphite controls, compact draft/article rows, inline totals and an adaptive site-status rail. Writing selects the destination site when needed; recent articles can be filtered by site and longer recent lists expanded.
- Refined the writing workspace with a narrower navigation/list rail, aligned title and body, compact view controls, and an on-demand tag/attachment panel that does not move the text. Publication details remain available in the workspace menu.

- Updated RSS, Atom and JSON Feed parsing to gofeed 1.5.0, preserving feeds with long preambles and rejecting invalid null JSON items without crashing.
- Updated the MyGo desktop framework, frontend runtime and build CLI together to 0.7.4, with the installer safety hook re-audited for that version.
- Windows CI uses shared quality and dependency security checks with pinned action revisions, and runs automatically on pull requests and pushes to main.
- Repository defaults now specify UTF-8 and consistent line endings, and exclude local credentials, Planet backup directories and build output from new commits.

### Fixed

- Keep installer auditing compatible with 7-Zip versions that expose NSIS-generated uninstallers, checking both copies for matching content while continuing to reject unknown or duplicate payload entries.
- Isolate release-test Git subprocesses from push-hook environment variables so checks in linked worktrees cannot modify the source repository's refs or configuration.
- Preserve a manually expanded article list while resizing the editor, and close article details with Escape without interfering with Chinese input composition or discarding pending tags.
- Aligned dashboard navigation and page headings, statistic labels and values, article columns, and site/service card content across desktop window sizes.
- Platinum template verification accepts LF and CRLF stylesheets while still rejecting changed content, so clean Git checkouts can use the bundled template.
- Service configuration accepts Windows short directory names while continuing to reject symbolic links and junctions in configuration paths.
