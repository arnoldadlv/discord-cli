---
status: accepted
---
# User token only, no bot support

The tool exists to search guilds and read DMs, and Discord exposes both only to user accounts: the message search endpoint and the DM list have no bot equivalents. DiscordChatExporter supports both token kinds, which costs it a second code path for threads and an intent check. We accept only a user token and will not add bot support.

## Consequences

- Automating a user account is against Discord's terms of service. Rate-limit handling is a safety feature, not a nicety, and the tool must never spray invalid requests.
- Every endpoint choice follows the user-account path (for example `channels/{id}/threads/search`, not `guilds/{id}/threads/active`).
- Primary sources for endpoint behavior are `legacy/node/src/client.js`, DiscordChatExporter's `DiscordClient.cs`, and the unofficial user API docs at docs.discord.food. Discord's official developer docs do not cover this surface.
