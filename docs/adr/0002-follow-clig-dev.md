---
status: accepted
---
# Follow the Command Line Interface Guidelines (clig.dev)

The Go port conforms to clig.dev rather than to the habits of the Node CLI it replaces. Chosen because the tool is used both by a person at a keyboard and by an agent through a skill, and clig resolves that tension with conventions the rest of the terminal already follows.

## Consequences

- Human-readable output when stdout is a TTY; `--json` on every command is the stable machine interface. Errors and progress go to stderr with mapped exit codes. Color and animation only on a TTY, and never when `NO_COLOR` is set.
- Argument parsing uses Cobra so that `-h` works anywhere, `discord help <command>` exists, flags are order-independent, and typos get suggestions. Everything else stays standard library, with one exception recorded in ADR-0006 (the SQLite driver for the search index).
- Secrets are not read from flags. The token comes from a credential file first (see ADR-0004); the `DISCORD_TOKEN` environment variable is accepted as a fallback with a notice.
- Interfaces are commitments from the Go port's first release onward (see ADR-0005 for the surface): changes are additive, deprecations are warned in the tool, and abbreviations of subcommands are never accepted.
- No analytics, no phoning home.
