---
name: discord-cli
description: >
  Use this skill whenever the user wants an agent to interact with their own
  Discord account through the `discord` command-line tool: reading recent
  messages, searching conversations, listing guilds, channels, or DMs,
  exporting chat history, checking what is being discussed, or searching
  existing exports on disk. Treat requests like "check Discord", "what are
  people saying about X", "search the server", "export that channel", "read
  the latest messages", or "what did someone post" as triggers. Skip for
  questions about Discord's own app, bot configuration, or server
  administration, where the user needs an explanation rather than an action.
---

# discord-cli

`discord` reads the user's own Discord account: guilds, channels, threads, DMs, live search, exports to JSON, and fast local search over those exports. It uses the account's own user token, never a bot. Source and README: https://github.com/arnoldadlv/discord-cli

This file matches the installed version. Re-install it after upgrading: `discord help --skill > ~/.claude/skills/discord-cli/SKILL.md`.

## Pre-flight

Before a long task, and whenever a command exits 3, check the token:

```bash
discord auth status --json
```

Exit 0 with `"valid": true` means Discord accepts the token. Exit 3 means there is no token or Discord rejected it. Stop and ask the user to run `discord auth set` (it prompts with echo off); do not try to obtain or store a token yourself.

Find the default guild with `discord config get --json`. If `default-guild` is empty, either pass `--guild <name>` on guild-scoped commands or ask the user which guild to set with `discord config set default-guild <name>`.

## Syntax

```bash
discord <noun> <verb> [argument] [flags]
```

Nouns: `guild`, `channel`, `dm`, `message`, `export`, `auth`, `config`, `cache`. Each command takes at most one positional argument (a channel or DM name, or a search query). Flags work in any order. `--help` works anywhere. Abbreviated command names are not accepted.

**Always pass `--json`.** Every command supports it, its field names are stable, and human output may change. Progress, notices, and errors go to stderr; stdout is only ever the answer.

### Exit codes

Branch on the code, not on error text.

| Code | Meaning | What to do |
|---|---|---|
| 0 | Success | Parse stdout |
| 1 | Unexpected error (network, timeout, a failed channel inside a guild export) | Read stderr; retry once for a timeout |
| 2 | Usage error (bad flag, missing argument, no guild and no default guild) | Fix the command; for "no guild" add `--guild <name>` |
| 3 | Authentication failed (no token, rejected token, bot token) | Stop and ask the user to run `discord auth set` |
| 4 | Guild, channel, DM, or message not found, or not visible to the account | stderr carries "Did you mean: ..."; pick one and retry. For a message id in no export, add `--channel` |
| 5 | No exports on disk | Run an export first |
| 6 | Rate limit exhausted | Wait a minute; lower `--concurrency` for exports |

### Names

Guilds, channels, and DMs are given by name or numeric id. Matching is case-insensitive, then normalised (spaces to hyphens, emoji and punctuation dropped), so `my-guild` finds `My Guild` and `news` finds `📰news`. No match exits 4 with up to five suggestions; several matches exit 4 listing the candidates, so use the id.

DMs are named by the other person's username or display name; group DMs by their name or any participant.

## Commands

| Command | Purpose | Key flags |
|---|---|---|
| `guild list` | Guilds the account belongs to | |
| `guild show` | One guild: counts and export status per channel | `--guild` |
| `guild search <query>` | Live search through Discord's search | `--guild`, `--channel`, `--has`, `--limit`, `--offset` |
| `guild export` | Export every text, announcement, and forum channel | `--guild`, `--threads`, `--full`, `--concurrency` |
| `channel list` | Channels grouped by category | `--guild`, `--threads` |
| `channel read <channel>` | Newest messages, oldest first, with embeds and reactions | `--guild`, `--limit`, `--threads` |
| `channel export <channel>` | Export one channel (and its threads with `--threads`) | `--guild`, `--full`, `--threads` |
| `dm list` | DMs and group DMs with participants | |
| `dm read <dm>` | Recent messages of a DM | `--limit` |
| `dm search <dm> --query <text>` | Search one DM by fetching its history | `--query`, `--after`, `--before`, `--limit` |
| `dm export <dm>` | Export a DM | `--full` |
| `message read <id>` | One message by id, with the messages around it | `--context`, `--channel`, `--guild` |
| `export list` | Every export on disk, all locations | `--guild` |
| `export search <query>` | Search exports on disk, unlimited | `--guild` or `--all`, `--author`, `--after`, `--before`, `--limit` |
| `auth set` / `auth status` | Store and check the token | |
| `config set default-guild <name>` / `config get` | The default guild | |
| `cache status` / `cache rebuild` / `cache clear` | The search index and lookup cache | |

Global flags: `--json`, `--format` (see below), `--width` (compact truncation, default 200), `--no-header` (tsv), `--no-cache` (bypass the 24 hour guild, channel, and DM lists), `--timeout` (per request, default 30s), `--no-color`.

## Output formats

Every command accepts `--format`: `human` (the default), `json` (the same as `--json`), `compact`, and `tsv`.

Use `json` for anything that needs the whole message: it names every field, including attachments, embeds, and reactions.

Use `compact` when the answer is a count, a group, or a scan through many lines rather than one message. It works on `channel read`, `dm read`, `export search`, `guild search`, and `message read`, printing one line per message in the same shape a grep result is, `path:line:content`:

```
guild-slug/channel-slug:message-id:timestamp:author: content
```

`guild-slug/channel-slug` is exactly the form `--guild` and a channel argument already accept, so `cut -d: -f1` hands back an address you can pass straight back to `channel read` or `message read`. Content is always one line: a newline becomes the two characters `\n`, a tab becomes `\t`, and it is cut off at `--width` characters (200 by default, 0 turns that off) with a trailing ellipsis. Attachments, embeds, and reactions never appear in compact; ask for `json` when a message's full detail matters.

Reach for `compact` over `json` whenever the question is about counting, grouping, or scanning many messages rather than reading one: how many messages mention a term, which channel talks about a topic the most, who posts the most, which day had the most activity. The answer is a distribution across many lines, not the content of any one message, so pipe compact lines through `cut`, `sort`, `uniq -c`, and `grep` the way `grep -n` output is used, instead of parsing JSON just to throw most of it away:

```bash
discord export search "access control" --guild my-guild --format=compact
discord export search "access control" --guild my-guild --format=compact | cut -d: -f1 | sort | uniq -c | sort -rn
```

Use `tsv` for the list commands: `guild list`, `channel list`, `dm list`, `export list`, and `cache status`. It is the same columns the human table shows, tab-separated, with a header row `--no-header` removes:

```bash
discord guild list --format=tsv --no-header
discord export list --format=tsv | cut -f1,3
```

## Quick reference

Read recent activity, oldest first. The JSON is a compact projection of Discord's message, not the raw object: each message has `id`, `timestamp`, `edited`, `author` (`id` and `name` only), `content`, `reply_to` (the id of the message it replies to, if any), `mentions`, `attachments`, `embeds`, and `reactions`, plus the resolved guild and channel. Empty arrays are left out. Twenty messages land well under 10 KB, where the raw objects would run past 50 KB:

```bash
discord channel read general --json
discord channel read news --limit 5 --json
discord dm read someone --limit 10 --json
```

Read one message by its id, the id every search hit carries. The id is looked up in the search index first, so a message already in an export comes back from disk with no network call, whichever guild, channel, or DM it is in. `--context N` adds the N messages either side, oldest first, and the message asked for carries `"match": true` (in human output, a leading `>` on its timestamp line). Fewer come back at the start or end of a channel. When no export holds the id there is nothing to look up, so `--channel` says where to fetch it from, and without it the command exits 4:

```bash
discord message read <id from a search hit> --json
discord message read <id from a search hit> --context 5 --json
discord message read <id from a search hit> --channel general --json
```

Search live, newest first. Limits above 25 are paged automatically. `--has` takes image, video, sound, file, embed, link, sticker, or poll. The JSON has `total_results`, `messages`, and `channel_names`:

```bash
discord guild search "access control" --json
discord guild search budget --channel general --limit 50 --json
discord guild search --query "policy" --has link --json
```

Search exports on disk, unlimited and repeatable. One of `--guild` or `--all` is required; `--guild dm` selects DM exports. Any query term matches, case-insensitive substring. The JSON has `total_matches`, `shown`, and `results` with `guild`, `channel`, `id`, `author`, `timestamp`, `content`, `file`:

```bash
discord export search "access control" --guild my-guild --json
discord export search policy --all --author someone --after 2026-01-01 --limit 100 --json
discord export search budget --guild dm --json
```

A search index makes this return in well under a second. If stderr says the index is missing or out of date, the search still works by scanning; run `discord cache rebuild` afterwards.

Discover:

```bash
discord guild list --json
discord guild show --json
discord channel list --threads --json
discord dm list --json
discord export list --json
```

Export. Runs are incremental; `--full` refetches; every export updates the search index. A guild export runs four channels at a time and exits 1 if any channel failed, keeping the others; rerun to retry:

```bash
discord guild export --json
discord guild export --threads --json
discord channel export general --json
discord dm export someone --json
```

## Patterns

Search, then read. A search returns an address, the `id` of every hit; the read expands one address into the conversation around it. Reach for this pair whenever a hit is worth more than its own line:

```bash
discord export search "onboarding" --guild my-guild --json
discord message read <id from a search hit> --context 5 --json
```

Research a topic: live first for what Discord indexes today, then local for depth.

```bash
discord guild search "onboarding" --limit 50 --json
discord export search "onboarding" --guild my-guild --limit 200 --json
```

If `export search` exits 5, run `discord guild export --json`, then search again.

Research a DM: `dm search` fetches the whole history every time and warns on stderr when it is long. Export once, then search the export:

```bash
discord dm export someone --json
discord export search "meeting" --guild dm --author someone --json
```

Refresh the archive before analysis:

```bash
discord guild export --json
discord export search "assessment" --guild my-guild --after 2026-01-01 --json
```

## Data locations

| Location | Contents |
|---|---|
| `~/.config/discord-cli/` | `token` (mode 0600) and `config.json` |
| `~/.local/share/discord-cli/exports/<guild>/` | New exports; threads under `threads/<channel>/`; DMs under `dm/` |
| `~/.cache/discord-cli/` | Search index and lookup cache; disposable |
| `~/.discord-cli/exports/<guild>/` | Exports from the old Node CLI; read and updated in place |
| `~/DiscordChatExporter.Cli.osx-arm64/exports/<guild>/` | DiscordChatExporter exports; read only |

Exports use the DiscordChatExporter envelope (`guild`, `channel`, `dateRange`, `messages`, `messageCount`) with raw API messages; each directory has a `.meta.json` keyed by channel id.

## Tips

1. `channel read` for recent activity with full embeds; `guild search` to find by keyword live; `export search` for unlimited, repeatable research; `message read` to open up one hit.
2. Emoji prefixes are optional in names; the id always works.
3. Exit 3: stop and ask the user about the token. Exit 4: read the suggestions. Exit 5: export first. Exit 6: wait, do not loop.
4. `--threads` on `channel list`, `channel read`, `channel export`, and `guild export` includes active and archived threads.
5. Timestamps in JSON are RFC 3339 as Discord returned them.
6. The old npm package `@poamslayer/discord-cli` is deprecated; its flat commands (`guilds`, `messages`, `search --local`) do not exist here.
7. `message read` and `export search` say on stderr whether the answer came from an export on disk or from Discord, and how stale an export answer might be. `source` in their JSON (`export` or `discord`) is the machine-readable form of the same fact; `guild search`, `channel read`, and `dm read` need no such note because their name already says where they read from.
