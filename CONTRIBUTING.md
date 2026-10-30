# Contributing

Contributions are welcome. Please discuss substantial changes in a [GitHub issue](https://github.com/puremixai/planet-windows/issues) before investing in a large implementation. Small fixes can be submitted directly as pull requests.

1. Fork and clone the repository, then create a branch.
2. Follow the build and test instructions in [README.md](README.md) and the detailed [Windows guide](windows/README.md).
3. Add or update focused tests when behavior changes. Run `bun test`, `go test ./...` and `bun run typecheck` from `windows/`; run `bun run build` when validating packaging changes.
4. Describe the problem, the change, test results and any limitations in your pull request. Do not include real backups, private keys, tokens or personal data in issues, tests or commits.

Compatibility changes should document which Planet backup formats and template revisions were tested. Never publish imported site data or IPNS keys as test fixtures. By contributing, you agree that your contributions may be distributed under the repository's MIT license.
