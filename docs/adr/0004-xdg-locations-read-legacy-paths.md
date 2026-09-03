---
status: accepted
---
# New files go to XDG directories; old locations stay readable

Per clig.dev we follow the XDG base directory spec. New exports are written under the XDG data directory (`~/.local/share/discord-cli/exports/<guild>/`) and the token lives in the XDG config directory (`~/.config/discord-cli/token`, mode 0600). The Node CLI wrote to `~/.discord-cli/exports/`, and thirteen exports plus the `searching-discord` skill depend on that path, so it is not moved: local search, info, and incremental export read it alongside the new location and the DiscordChatExporter folder, exactly as the Node CLI already read two locations.

## Consequences

- Three read locations, one write location. A guild's exports may be split across old and new directories; incremental export must find the existing file wherever it is before deciding to write a new one.
- The `searching-discord` skill's data-location table needs updating when the port ships.
