# discord-cli

A command-line tool that reads a person's own Discord account: the guilds they belong to, the channels and direct messages they can see, and the messages in them. It exists so that a person, or an agent acting for them, can search and archive Discord conversations from a terminal.

## Language

**Guild**:
A Discord community the account belongs to. Discord's UI calls this a "server"; the API and this project call it a guild.
_Avoid_: Server (except in human-facing prose)

**Channel**:
A named message stream inside a guild. Includes text, announcement, and forum channels; excludes voice.
_Avoid_: Room

**Thread**:
A message stream that hangs off a channel. May be active or archived.

**DM**:
A direct-message conversation between the account and one or more people, outside any guild. A DM with several people is a group DM and is still a DM.
_Avoid_: Private channel, private message, conversation, chat

**Default guild**:
The guild a command acts on when none is named. Set once in the tool's configuration.

**Message**:
One post in a channel, thread, or DM, as Discord's API returns it: content, author, timestamp, attachments, embeds, reactions.

**User token**:
The credential of the account itself, as the official Discord client presents it. Not a bot token. The only kind of credential this project accepts.
_Avoid_: API key, bot token

**Export**:
A local JSON file holding every message of one channel, thread, or DM, plus the guild and channel it came from and the date range covered.
_Avoid_: Dump, backup, archive (verb is fine: "to export")

**Incremental export**:
An export run that fetches only messages newer than the last one already on disk and merges them into the existing export.
_Avoid_: Sync, update

**Export meta**:
The per-guild record of where each channel's last export stopped, used to make the next export incremental.
_Avoid_: Cursor file, state file

**Live search**:
A search that asks Discord's search endpoint and returns whatever it currently indexes.
_Avoid_: Online search, API search, remote search

**Local search**:
A search that scans exports on disk. Unlimited in size, but only as fresh as the last export.
_Avoid_: Offline search, file search

**Native export**:
An export written by this project. Messages are stored exactly as the API returned them.

**Legacy export**:
An export written by DiscordChatExporter, which stores messages in its own shape. This project reads them but never writes them.
_Avoid_: DCE export, old export

**Search index**:
A disposable full-text index built from the exports on disk so local search is fast. Deleting it loses nothing; it is rebuilt from the exports.
_Avoid_: Database, cache DB

**Lookup cache**:
A short-lived local copy of the guild, channel, and DM lists, used to turn names into ids without asking Discord every time.
