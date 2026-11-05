# Repository instructions

- Use PowerShell for terminal commands and UTF-8 for text files.
- This is the Windows port. The application lives in `windows/` and uses Go, MyGo, WebView2 and TypeScript.
- Read `CONTRIBUTING.md` and `docs/git-workflow.md` before changing the development workflow.
- Inspect `git status` first. Preserve existing work and use a short-lived topic branch for changes.
- Keep commits focused; separate mechanical formatting from behavior changes. Use the commit types documented in the workflow guide.
- Install frontend dependencies with `bun install --frozen-lockfile` from `windows/`. Keep `bun.lock` and `go.sum` tracked.
- Run `bun run check` from `windows/` for Go formatting, static analysis, frontend and Go tests, TypeScript checks and the frontend production build. CI uses the same entry point.
- For packaging changes, also run `bun run build` and the relevant checks in `windows/docs/release-audit.md`. The shared check does not validate an installer.
- Optional Kubo, online and external-template tests require explicit environment setup; report which were actually run.
- Keep credentials, real Planet backups, IPNS private keys and build output out of commits and test fixtures.
- The app version comes from `windows/release.json`. Keep release tags and changelog headings consistent with it; do not introduce another version constant.
- Add user-visible changes to `CHANGELOG.md` under `Unreleased`. Do not invent release dates or imply that an untagged development snapshot is a published release.
