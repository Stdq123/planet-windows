# Security policy

This project is in early development and does not currently promise a supported release or response schedule. Please avoid publishing suspected vulnerabilities, credentials or private keys in public issues.

Use [Report a vulnerability](https://github.com/Stdq123/planet-windows/security/advisories/new), which is enabled for this repository. Include the affected revision, impact and a minimal reproduction with synthetic data. If the private channel is temporarily unavailable, contact a repository maintainer privately through a channel you already have; do not include sensitive details in a public issue.

Backups exported by the application contain unencrypted IPNS private keys. Treat them as secrets. IPFS publication makes site content public and cannot guarantee that published content can later be removed from every node.

Maintainers run `bun run security:check` from `windows/` for dependency changes and before release. It checks Bun dependencies and reachable Go vulnerabilities using online advisory databases; it does not cover bundled Kubo or template assets. Network failures and findings must be investigated, not silently skipped. See the [maintenance standard](docs/maintenance.md) and [release process](docs/releasing.md) for scope and release gates.
