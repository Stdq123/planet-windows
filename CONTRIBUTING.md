# Contributing

Contributions are welcome. Please discuss substantial changes in a [GitHub issue](https://github.com/Stdq123/planet-windows/issues) before investing in a large implementation. Small fixes can be submitted directly as pull requests.

1. Fork and clone the repository, then create a short-lived branch from an up-to-date `main`. Use `feature/`, `fix/`, `chore/` or `refactor/` followed by a short description.
2. Follow the build and test instructions in [README.md](README.md) and the detailed [Windows guide](windows/README.md).
3. Add or update focused tests when behavior changes. After `bun install --frozen-lockfile`, run `bun run check` from `windows/`. This is also the CI entry point: Go formatting, `go vet`, frontend and Go tests, TypeScript checks and the frontend production build. Run `bun run build` and the relevant release audits for packaging changes.
4. Keep commits focused. Use `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `style:` or `chore:` titles; explain the reason in the body when helpful. Separate mechanical formatting from behavior changes. Review the staged diff before committing.
5. Describe the problem, the change, test results and any limitations in your pull request. Add user-visible changes to [CHANGELOG.md](CHANGELOG.md) under `Unreleased`. Do not include real backups, private keys, tokens or personal data in issues, tests or commits.

The [Git workflow guide (Chinese)](docs/git-workflow.md) covers optional Git hooks, branch and commit practices, release tags, rollback and mirror synchronization. Files use UTF-8 and the line endings defined by `.editorconfig` and `.gitattributes`. Keep dependency lock files tracked; update them with the corresponding package manager.

Compatibility changes should document which Planet backup formats and template revisions were tested. Never publish imported site data or IPNS keys as test fixtures. By contributing, you agree that your contributions may be distributed under the repository's MIT license.
