# discord-cli

A command-line tool that reads your own Discord account: the guilds you belong to, the channels and DMs you can see, and the messages in them. You can search live through Discord, export channels and DMs to JSON files, and search those files on your own machine without limits. It works for a person at the keyboard and for an agent that calls it with `--json`.

It talks to Discord as your own account, with your user token. It never uses a bot token.

## Install

You need Go 1.25 or newer. No C compiler is needed.

```sh
go install github.com/arnoldadlv/discord-cli/cmd/discord@latest
```

The binary is named `discord`. Make sure `$(go env GOPATH)/bin` is on your `PATH`.

## Set your token

Get your user token from the Discord web client (DiscordChatExporter's guide explains how). Then store it in a file only you can read:

```sh
discord auth set
```

The prompt hides what you paste. If you would rather pipe it in, that works too and keeps the token out of your shell history:

```sh
printf '%s' "$TOKEN" | discord auth set
```

Check that Discord accepts it:

```sh
discord auth status
```

The token file lives in your config directory (see Data locations). `DISCORD_TOKEN` in the environment still works as a fallback; the tool prints a one-line notice suggesting `auth set` when it uses it. The tool refuses bot tokens.

## Set a default guild

Most commands act on one guild. Set it once and leave `--guild` off afterwards:

```sh
discord config set default-guild "My Guild"
discord config get
```

`--guild` on any command beats the default. Names are matched case-insensitively, and the hyphenated form works: `my-guild` finds `My Guild`.

## A tour

Commands are noun then verb. Every command accepts `--json`, and progress and errors go to stderr so stdout is only ever the answer.

See what you have access to:

```sh
discord guild list
discord channel list
discord channel list --threads
discord dm list
```

Read a channel or a DM. The newest messages come oldest first, with attachments, embeds, and reactions:

```sh
discord channel read general
discord channel read news --limit 5
discord dm read kyle
```

Channel names can leave the emoji off: `news` finds `📰news`. A typo gets a "did you mean" with the closest names.

Search live through Discord:

```sh
discord guild search "access control"
discord guild search MFA --channel general --limit 50
discord guild search --query "policy" --has link --json
```

Discord has no search for DMs, so `dm search` fetches the DM's history and filters it. For repeated research, export the DM once and search the export:

```sh
discord dm search kyle --query "meeting"
```

Export to JSON. Exports are incremental: the second run fetches only what is new. `--full` refetches everything. `--threads` exports threads as their own files.

```sh
discord guild export
discord guild export --threads --concurrency 2
discord channel export general
discord dm export kyle
```

Search your exports on your own machine, without limits. Say where to search with `--guild` or `--all`:

```sh
discord export list
discord export search "assessment" --guild my-guild
discord export search policy --all --author kyle --after 2026-01-01
discord export search MFA --guild dm --json
```

Every export updates a search index, so local search returns in well under a second. If the index is missing or out of date the search still works, just slower, and says so on stderr. `discord cache status` shows the state of the index; `discord cache rebuild` rebuilds it; `discord cache clear` deletes it and the lookup cache and nothing else.

Get help anywhere:

```sh
discord
discord --help
discord help guild search
discord channel read -h
```

## Data locations

The tool follows the XDG base directory rules. Nothing you already have is moved.

| What | Where |
|---|---|
| Token file (mode 0600) and `config.json` | `~/.config/discord-cli/` |
| New exports | `~/.local/share/discord-cli/exports/<guild>/<channel>.json`, threads under `threads/<channel>/`, DMs under `dm/` |
| Search index and lookup cache (disposable) | `~/.cache/discord-cli/` |
| Exports written by the old Node CLI (read, and updated in place by incremental export) | `~/.discord-cli/exports/<guild>/` |
| Exports written by DiscordChatExporter (read only, never modified) | `~/DiscordChatExporter.Cli.osx-arm64/exports/<guild>/` |

Set `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, or `XDG_CACHE_HOME` to move the first three.

Exports use the DiscordChatExporter envelope (`guild`, `channel`, `dateRange`, `messages`, `messageCount`) with each message stored exactly as Discord's API returned it. A `.meta.json` file in each export directory records where the last export stopped, per channel id.

The guild, channel, and DM lists are cached for 24 hours so names resolve without asking Discord. `--no-cache` bypasses that.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unexpected error |
| 2 | Usage error (bad flag, missing argument, no guild and no default guild) |
| 3 | Authentication failed (no token, rejected token, bot token) |
| 4 | Guild, channel, or DM not found |
| 5 | No exports found |
| 6 | Rate limit exhausted |

## A caution about your account

Automating a user account is against Discord's terms of service and can get an account terminated. This tool exists for reading your own account, and you use it at your own risk.

The tool is careful on your behalf. Every request carries the same headers the official web client sends. On a rate limit it sleeps for the time Discord asks and gives up after five attempts (exit code 6). It honours the advisory rate-limit headers with a buffer. It never retries a request Discord rejected as unauthorised or forbidden, because too many of those get an IP banned. Guild exports run four channels at a time by default; lower `--concurrency` if you want to be gentler.

Nothing leaves your machine except requests to Discord. There are no analytics.

## Uninstall

```sh
rm "$(go env GOPATH)/bin/discord"
rm -r ~/.config/discord-cli ~/.cache/discord-cli
```

Delete `~/.local/share/discord-cli` too if you do not want to keep the exports.

## Development

```sh
go test ./...
go vet ./...
gofmt -l .
```

Tests drive the tool through its command boundary against a fake Discord server and a temporary home. They contain only invented names and messages. One test talks to the real API and is skipped unless you opt in:

```sh
DISCORD_CLI_LIVE_TEST=1 DISCORD_TOKEN=... go test ./internal/smoke/
```

CI runs the format check, vet, the tests, and a `CGO_ENABLED=0` build on every pull request.

The glossary is in `CONTEXT.md` and the design decisions are under `docs/adr/`. The Node.js CLI this replaces is kept under `legacy/node/` for reference.

## License

MIT. See `LICENSE`.
