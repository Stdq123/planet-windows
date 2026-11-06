# Changelog

User-facing changes are recorded here. Release entries use `MAJOR.MINOR.PATCH` and an immutable `vMAJOR.MINOR.PATCH` Git tag. Earlier development snapshots are described in the [Windows guide](windows/README.md); this file does not reconstruct unverified release dates.

## [Unreleased]

### Security

- Updated Markdown rendering to reject encoded and automatic script URLs ([GO-2026-5320](https://pkg.go.dev/vuln/GO-2026-5320)), with a regression test covering links and images.
- Updated the HTML parser and renderer to fix reachable issues in untrusted feed and preview content ([GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860), [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866)).

### Added

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

- Renamed the Chinese overview navigation, heading, refresh label and loading/error messages to “写作台”.
- Rebuilt the all-sites overview from the approved Figma writing workspace, with the subsequent neutral-palette revision: warm white surfaces, graphite controls, compact draft/article rows, inline totals and an adaptive site-status rail. Writing selects the destination site when needed; recent articles can be filtered by site and longer recent lists expanded.
- Refined the writing workspace with a narrower navigation/list rail, aligned title and body, compact view controls, and an on-demand tag/attachment panel that does not move the text. Publication details remain available in the workspace menu.

- Updated RSS, Atom and JSON Feed parsing to gofeed 1.5.0, preserving feeds with long preambles and rejecting invalid null JSON items without crashing.
- Updated the MyGo desktop framework, frontend runtime and build CLI together to 0.7.4, with the installer safety hook re-audited for that version.
- Windows CI uses shared quality and dependency security checks with pinned action revisions, and runs automatically on pull requests and pushes to main.
- Repository defaults now specify UTF-8 and consistent line endings, and exclude local credentials, Planet backup directories and build output from new commits.

### Fixed

- Preserve a manually expanded article list while resizing the editor, and close article details with Escape without interfering with Chinese input composition or discarding pending tags.
- Aligned dashboard navigation and page headings, statistic labels and values, article columns, and site/service card content across desktop window sizes.
- Platinum template verification accepts LF and CRLF stylesheets while still rejecting changed content, so clean Git checkouts can use the bundled template.
- Service configuration accepts Windows short directory names while continuing to reject symbolic links and junctions in configuration paths.
