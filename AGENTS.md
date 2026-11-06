# Repository instructions

- Use PowerShell for terminal commands and UTF-8 for text files.
- This is the Windows port. The application lives in `windows/` and uses Go, MyGo, WebView2 and TypeScript.
- Read `CONTRIBUTING.md` and `docs/git-workflow.md` before changing the development workflow.
- Inspect `git status` first. Preserve existing work and use a short-lived topic branch for changes.
- Keep commits focused; separate mechanical formatting from behavior changes. Use the commit types documented in the workflow guide.
- Install frontend dependencies with `bun install --frozen-lockfile` from `windows/`. Keep `bun.lock` and `go.sum` tracked.
- Run `bun run check` from `windows/` for Go formatting, static analysis, frontend and Go tests, TypeScript checks and the frontend production build. CI uses the same entry point.
- For changes limited to the documentation allowlist in `scripts/check-changes.ts`, run `bun run check:docs` instead; it needs only Git and Bun. CI and pre-push classify actual Git changes and use this lightweight check without installing Go or frontend dependencies. Mixed changes, release metadata, license/embedded assets, automation changes and uncertain comparisons still require full checks. Manual CI runs always use full checks. See `docs/git-workflow.md` for scope and push prerequisites.
- Windows CI is enabled for pull requests and pushes to main, with manual dispatch available. Keep the shared quality checks and dependency security scan passing; do not bypass failures to merge. Preserve read-only permissions, pinned actions, concurrency cancellation and the 30-minute timeout.
- Dependabot version and security update PRs are enabled. Keep weekly version checks capped at three open PRs per ecosystem, automatic rebasing disabled and automatic merging off. Review and verify dependency updates locally and in CI before merging.
- Run `bun run security:check` for dependency changes and release preparation; online advisory/tool failures are failures, not a clean audit.
- Follow `docs/releasing.md` for release preparation. Default output is an unapproved draft. Stable releases require valid first-party Authenticode signatures and complete applicable native Windows acceptance. An explicitly chosen unsigned prerelease may use `-Release -AllowUnsignedPrerelease`, with the generated unsigned notice and SHA-256 checksums. This option does not waive metadata, license or the applicable native acceptance requirements, and never accepts invalid or untrusted signatures.
- For `v0.0.1-alpha.1` only, the maintainer has authorized unsigned publication with the disclosed limited native acceptance scope in `docs/acceptance-0.0.1-alpha.1.md`. Preserve every untested item and the stable-release requirements. Rebuild and rerun checks and native smoke from the final tagged commit; publish the final package hashes and evidence as Release attachments, without writing a self-referential final commit hash into this record. No production update feed is configured; this alpha uses manual downloads, so production feed publication is not applicable. Do not add a template-review or signing gate to this authorized alpha, or describe candidate evidence as proof of an unbuilt final package.
- For packaging changes, also run `bun run build` and the relevant checks in `windows/docs/release-audit.md`. The shared check does not validate an installer.
- Optional Kubo, online and external-template tests require explicit environment setup; report which were actually run.
- Keep credentials, real Planet backups, IPNS private keys and build output out of commits and test fixtures.
- The app version comes from `windows/release.json`. Keep release tags and changelog headings consistent with it; do not introduce another version constant.
- Add user-visible changes to `CHANGELOG.md` under `Unreleased`. Do not invent release dates or imply that an untagged development snapshot is a published release.
