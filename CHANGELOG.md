# Changelog

User-facing changes are recorded here. Release entries use `MAJOR.MINOR.PATCH` and an immutable `vMAJOR.MINOR.PATCH` Git tag. Earlier development snapshots are described in the [Windows guide](windows/README.md); this file does not reconstruct unverified release dates.

## [Unreleased]

### Security

- Updated Markdown rendering to reject encoded and automatic script URLs ([GO-2026-5320](https://pkg.go.dev/vuln/GO-2026-5320)), with a regression test covering links and images.
- Updated the HTML parser and renderer to fix reachable issues in untrusted feed and preview content ([GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860), [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866)).

### Added

- A single `bun run check` command for Go formatting and static analysis, frontend and Go tests, TypeScript checks and the frontend production build.
- Optional Git hooks to catch staged whitespace errors before commits and run the shared checks before pushes.
- A Chinese Git workflow guide covering branches, focused commits, pull requests, versioning and mirror synchronization.
- Weekly dependency update proposals for GitHub Actions, Go modules and Bun packages.

### Changed

- Windows CI now uses the same checks as local development, includes the frontend production build, and cancels superseded runs.
- Repository defaults now specify UTF-8 and consistent line endings, and exclude local credentials, Planet backup directories and build output from new commits.
