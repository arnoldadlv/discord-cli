# discord-cli

A command-line tool that reads your own Discord account: the guilds you belong to, the channels and DMs you can see, and the messages in them. You can run a live search through Discord, export channels and DMs to JSON files, and run a local search over those files on your own machine without limits. It works for a person at the keyboard and for an agent that calls it with `--json`.

It talks to Discord as your own account, with your user token. It never uses a bot token.

## Install

Pick one of the two ways.

### With the install script

This works on macOS and Linux and does not need Go. It downloads the binary for your machine from the latest GitHub release, checks it against the release's checksums, and puts it in `~/.local/bin`.

```sh
curl -fsSL https://raw.githubusercontent.com/arnoldadlv/discord-cli/main/install.sh | bash
```

If `~/.local/bin` is not on your `PATH`, the script prints the line to add. Set `DISCORD_INSTALL_DIR` to install somewhere else, or `DISCORD_VERSION=v0.1.0` to pin a release. Windows binaries are on the [releases page](https://github.com/arnoldadlv/discord-cli/releases).

### With Go

You need Go 1.25 or newer. No C compiler is needed.

1. Install the binary:

   ```sh
   go install github.com/arnoldadlv/discord-cli/cmd/discord@latest
   ```

   The binary is named `discord`. Go puts it in `$(go env GOPATH)/bin`, which is usually `~/go/bin`.

2. Put that directory on your `PATH`. It often is not. If you see `command not found: discord`, this is the reason.

   For zsh, add this line to `~/.zshrc`:

   ```sh
   export PATH="$HOME/go/bin:$PATH"
   ```

   For bash, add the same line to `~/.bashrc`:

   ```sh
   export PATH="$HOME/go/bin:$PATH"
   ```

   Then open a new terminal, or run `source ~/.zshrc` or `source ~/.bashrc` in the current one.

### Check that it runs

```sh
discord --version
```

This prints the version, the Go version it was built with, and the commit.

## Upgrade

Run the same install line again. The script replaces the binary with the latest release. With Go:

```sh
go install github.com/arnoldadlv/discord-cli/cmd/discord@latest
```

The Go module proxy caches "latest" for a few minutes. If you want a commit that was merged moments ago, skip the proxy and name the branch:

```sh
GOPROXY=direct go install github.com/arnoldadlv/discord-cli/cmd/discord@main
```

After an upgrade, re-install the agent skill if you use it (see Agent skill).

## First run

### Set your token

The tool needs your user token. This is the credential your own Discord account uses. You can copy it from the Discord web client: open your browser's developer tools, look at the network requests the client makes, and read the `Authorization` header from one of them. DiscordChatExporter's [token guide](https://github.com/Tyrrrz/DiscordChatExporter/blob/master/.docs/Token-and-IDs.md) shows each step with screenshots.

Store it in a file only you can read:

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

The token file lives in your config directory (see Data locations). `DISCORD_TOKEN` in the environment still works as a fallback. The tool prints a one-line notice suggesting `auth set` when it uses it. Bot tokens are refused with exit code 3.

### Set a default guild

Most commands act on one guild. Set it once and leave `--guild` off afterwards:

```sh
discord config set default-guild "My Guild"
discord config get
```

`--guild` on any command beats the default. Names are matched case-insensitively, and the hyphenated form works: `my-guild` finds `My Guild`.

### Make your first export

The best first command is a full export of the guild, threads included:

```sh
discord guild export --threads
```

This writes one JSON file per channel and one per thread. Exports are incremental: the next run fetches only the messages that are new since the last one and merges them into the file on disk. The run is also resumable. The export meta is written after each channel finishes, so if you stop a run, the next run keeps the finished channels and carries on from there.

If you have exports from the old Node CLI under `~/.discord-cli/exports`, the incremental export finds them there and merges new messages into them in place. Exports written by DiscordChatExporter are read but never written to.

Every export also refreshes the search index, so `discord export search` is fast right away.

## Search index

Local search reads a search index built from the exports on disk. Each export refreshes it. If you already have a large set of exports, for example from DiscordChatExporter, build the index once:

```sh
discord cache rebuild
```

On a 1.6 GB set of exports holding about 700,000 messages this took about 35 seconds. After that, searches return in well under a second.

A file that an interrupted DiscordChatExporter run left cut off cannot be parsed. The rebuild skips it and says so. `discord cache status` lists such files under "unreadable". Local search skips them too. If you want that channel searchable, export it again.

If the index is missing or out of date, local search still works by scanning the files, just slower, and says so on stderr. `discord cache clear` deletes the index and the lookup cache and nothing else. The index is disposable and is rebuilt from the exports.

## A tour of the commands

Commands are noun then verb. Every command accepts `--json`, and progress and errors go to stderr so stdout is only ever the answer.

See what you have access to:

```sh
discord guild list
discord guild show
discord channel list
discord channel list --threads
discord dm list
```

Read a channel or a DM. The newest messages come oldest first, with attachments, embeds, and reactions:

```sh
discord channel read general
discord channel read news --limit 5
discord dm read someone
```

Channel names can leave the emoji off: `news` finds `📰news`. A typo gets a "did you mean" with the closest names.

Live search through Discord:

```sh
discord guild search "access control"
discord guild search MFA --channel general --limit 50
discord guild search --query "policy" --has link --json
```

Discord has no search for DMs, so `dm search` fetches the DM's history and filters it. For repeated research, export the DM once and run a local search over the export:

```sh
discord dm search someone --query "meeting"
```

Export to JSON. Exports are incremental: the second run fetches only what is new. `--full` refetches everything. `--threads` exports threads as their own files.

```sh
discord guild export
discord guild export --threads --concurrency 2
discord channel export general
discord dm export someone
```

Local search over your exports, without limits. Say where to search with `--guild` or `--all`:

```sh
discord export list
discord export search "assessment" --guild my-guild
discord export search policy --all --author someone --after 2026-01-01
discord export search MFA --guild dm --json
```

Look after the search index and the lookup cache:

```sh
discord cache status
discord cache rebuild
discord cache clear
```

Get help anywhere:

```sh
discord
discord --help
discord help guild search
discord channel read -h
```

## Agent skill

The binary ships its own skill, a Markdown file that tells a coding agent how to use the tool. Print it with:

```sh
discord help --skill
```

To install it for Claude Code:

```sh
mkdir -p ~/.claude/skills/discord-cli
discord help --skill > ~/.claude/skills/discord-cli/SKILL.md
```

Codex and other agents that follow the Agent Skills standard load skills from `~/.agents/skills` per user or `.agents/skills` per repository:

```sh
mkdir -p ~/.agents/skills/discord-cli
discord help --skill > ~/.agents/skills/discord-cli/SKILL.md
```

The file is a snapshot of the skill for the installed version. Re-install it after upgrading the tool.

## Data locations

The tool follows the XDG base directory rules. Nothing you already have is moved. There are three places it reads exports from and one place it writes them to.

| What | Where |
|---|---|
| Token file (mode 0600) and `config.json` | `~/.config/discord-cli/` |
| New exports (the one write location) | `~/.local/share/discord-cli/exports/<guild>/<channel>.json`, threads under `threads/<channel>/`, DMs under `dm/` |
| Search index and lookup cache (disposable) | `~/.cache/discord-cli/` |
| Exports written by the old Node CLI (read, and merged into in place by incremental export) | `~/.discord-cli/exports/<guild>/` |
| Exports written by DiscordChatExporter (read only, never modified) | `~/DiscordChatExporter.Cli.osx-arm64/exports/<guild>/` |

Set `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, or `XDG_CACHE_HOME` to move the first three.

Native exports use the DiscordChatExporter envelope (`guild`, `channel`, `dateRange`, `messages`, `messageCount`) with each message stored exactly as Discord's API returned it. Legacy exports written by DiscordChatExporter keep their own message shape, and local search understands both. A `.meta.json` file in each export directory records where the last export stopped, per channel id.

The guild, channel, and DM lists are kept in the lookup cache for 24 hours so names resolve without asking Discord. `--no-cache` bypasses that.

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

The tool is careful on your behalf. Every request carries the same headers the official web client sends. On a rate limit it sleeps for the time Discord asks and gives up after five attempts (exit code 6). It honours the advisory rate-limit headers with a buffer. It never retries a request Discord rejected as unauthorised or forbidden, because too many of those get an IP banned. Guild exports run four channels at a time by default. Lower `--concurrency` if you want to be gentler.

Nothing leaves your machine except requests to Discord. There are no analytics.

## Uninstall

```sh
rm ~/.local/bin/discord            # installed with the script
rm "$(go env GOPATH)/bin/discord"  # installed with Go
rm -r ~/.config/discord-cli ~/.cache/discord-cli
```

Delete `~/.local/share/discord-cli` too if you do not want to keep the exports. Delete the `SKILL.md` files under `~/.claude/skills/discord-cli` and `~/.agents/skills/discord-cli` if you installed the agent skill.

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

CI runs `gofmt -l .`, `go vet ./...`, `go test -race ./...`, and a `CGO_ENABLED=0 go build ./...` on every pull request. Pushing a tag like `v0.1.0` builds the release binaries for macOS, Linux, and Windows on amd64 and arm64, with a `checksums.txt`, and publishes a GitHub Release. The install script's tests run it against a local fake release server.

The glossary is in `CONTEXT.md` and the design decisions are under `docs/adr/`. The Node.js CLI this replaces is kept under `legacy/node/` for reference.

## License

MIT. See `LICENSE`.
