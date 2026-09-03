---
status: accepted
---
# Local search uses a SQLite full-text index derived from the JSON exports

Local search over the existing exports (1.6 GB, about 700,000 messages as of 2026-09-03) takes seconds per query when every file is re-read. We add a SQLite database with full-text search (FTS5) in the XDG cache directory, built from the JSON exports, updated after each export, and rebuilt on demand. The JSON exports remain the source of truth (ADR-0003); the index is disposable and `discord cache clear` deletes it without losing anything.

We considered making the database the primary store and dropping JSON. Rejected: the JSON format is what DiscordChatExporter tooling and the `searching-discord` skill read, and a cache that can be thrown away is simpler to reason about than a second copy of the truth.

We also considered a read-through cache in front of every command. Rejected: `channel read` and `guild search` are about what Discord has now, and clig.dev says talking to the network should be explicit rather than a background side effect. Only local search and name resolution read from disk first.

## Consequences

- One non-standard-library dependency beyond Cobra: the pure-Go SQLite driver (no C compiler), so `go install` stays a one-liner. ADR-0002 is amended accordingly.
- Guild, channel, and DM lists are cached for 24 hours for name resolution, with `--no-cache` to bypass. Fewer requests against a user account is part of the safety story in ADR-0001.
- The JSON scan is kept as the fallback path while the index is missing or rebuilding, and ships first.
