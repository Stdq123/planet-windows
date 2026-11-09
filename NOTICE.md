# Third-party notices

Planet Windows is derived from work in [Planetable/Planet](https://github.com/Planetable/Planet), which is distributed under the MIT license. The original copyright and license notice is reproduced in [LICENSE](LICENSE).

The application depends on third-party Go and JavaScript packages and packages Kubo in Windows builds. These components have their own licenses. The build script copies Kubo's license files into the bundled resources. Distributors should review and include the applicable notices for all components before publishing binaries; this file does not represent a completed third-party license audit.

Release materials include a corresponding source archive with vendored Go dependencies and the pinned go-ethereum library source. The library's LGPL-3.0 terms are retained alongside its source. See [rebuilding a release](docs/rebuilding-release.md) for rebuilding the application with a modified library. The project's MIT license does not replace any dependency's license.
