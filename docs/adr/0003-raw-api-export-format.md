---
status: accepted
---
# Exports store messages exactly as the API returns them

A native export is the DiscordChatExporter envelope (`guild`, `channel`, `dateRange`, `messages`, `messageCount`) with each message stored as the raw API object. We considered converting to DiscordChatExporter's own message shape so all exports on disk would share one dialect, and rejected it: the conversion is lossy, and the thirteen existing native exports already use the raw shape.

## Consequences

- Legacy exports written by DiscordChatExporter are read but never written or converted.
- Local search must understand both dialects, at minimum the author field (`author.nickname`, `author.name`, `author.username`) and timestamps in both offset styles.
