---
status: accepted
---
# Noun-verb command surface, a clean break from the Node CLI

The Node CLI had seven flat commands (`guilds`, `channels`, `dms`, `messages`, `search`, `export`, `info`) that mixed nouns and verbs and hid DMs behind flags. The Go port uses `discord <noun> <verb>` with four nouns from the glossary (`guild`, `channel`, `dm`, `export`) and the same verbs across them (`list`, `show`, `read`, `search`, `export`), plus `auth` and `config`. Per clig.dev this is the common shape for several objects with several operations, and consistent verbs make commands guessable.

We considered keeping the flat surface so the `searching-discord` skill would not change. Rejected: the Node CLI is uninstalled, the skill is being rewritten for the port anyway, and there are no other users, so the break is free now and expensive later. The interface commitment in ADR-0002 begins with the Go surface.

## Consequences

- Each command takes at most one positional argument, the primary object (a channel or DM name). Guild is a flag, `--guild`, which falls back to a configured default guild. Flags beat config.
- Live search lives under `guild` because the endpoint is guild-wide; local search lives under `export` because exports are what it reads.
- `dm export` is new; the Node CLI could not export DMs.
