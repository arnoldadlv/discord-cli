# Legacy Node.js discord-cli (v1.0.0)

**Deprecated.** The Go tool at the root of this repository replaces it:
`go install github.com/arnoldadlv/discord-cli/cmd/discord@latest`. The npm
package is to be marked deprecated with this command, run by the maintainer
once logged in to npm:

    npm deprecate @poamslayer/discord-cli@1.0.0 "Replaced by the Go tool: go install github.com/arnoldadlv/discord-cli/cmd/discord@latest"

This is the original zero-dependency Node.js CLI, recovered from the published
npm package `@poamslayer/discord-cli@1.0.0` on 2026-09-03 after the source was
lost from local git and the GitHub remote.

It is kept here as the primary source for the Go port. It is not maintained.

Contents are exactly what the npm tarball shipped (`bin/` and `src/`). The
original README, tests, and git history were not part of the package and were
not recoverable.

Do not install it. If you need to run it for comparison, `npm link` from this
directory works (Node 18+ and `DISCORD_TOKEN`).
