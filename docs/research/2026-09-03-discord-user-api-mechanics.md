# Discord user-account API mechanics the Go port must replicate

Date: 2026-09-03
Question: what does the legacy Node CLI under `legacy/node/` do on the wire, on disk, and in name resolution, so a Go port can reproduce it, and where does it differ from DiscordChatExporter and from the unofficial user API docs.

Every claim below is tied to a file and line, or to a URL and quoted text. Where I inferred something instead of reading it, the sentence starts with "Inference:". Line numbers for the unofficial docs refer to the local copies I fetched with `defuddle` (see Sources), and for the raw MDX source files from GitHub.

## Summary

- The CLI authenticates with a raw user token in `Authorization` (no `Bot ` prefix) and sends ten more headers that imitate Chrome on macOS, including a base64 `X-Super-Properties` JSON blob (client.js:32-45). DiscordChatExporter sets only `Authorization` in its client class and works with user tokens (DiscordClient.cs:41-44). Only `Authorization` is proven required.
- All requests go to `https://discord.com/api/v10` with query parameters and GET only (client.js:1, 47-57). There is no POST anywhere in the CLI.
- Message listing uses `limit=100` plus `before` or `after`, and pages backward by setting `before` to the last id of each page (client.js:127-144). Guild search uses `/guilds/{id}/messages/search` with `content`, `channel_id`, `has`, `limit` capped at 25, `offset`, `sort_by=timestamp`, `sort_order=desc`, and reads `messages` as an array of arrays plus `total_results` (client.js:148-159, search.js:34-59).
- The CLI calls `/guilds/{id}/threads/active`, which the unofficial docs say "is not usable by user accounts" (raw-channel.mdx:1251-1255). The CLI swallows the error, so `--threads` silently yields zero threads. DiscordChatExporter uses `/channels/{id}/threads/search` for user tokens instead (DiscordClient.cs:451-471).
- On 429 the CLI sleeps `retry_after` seconds (default 1) and retries once. After a success with `x-ratelimit-remaining: 0` it sleeps `x-ratelimit-reset-after` seconds (client.js:54-81). DiscordChatExporter does the same but adds 1 second and caps at 60 seconds (DiscordClient.cs:75-86).
- Names normalize by lowercasing, replacing whitespace runs with `-`, and deleting every character outside `[a-z0-9-]` (store.js:8-10, client.js:175). Guild matching accepts the exact lowercase name or the normalized name, with "Did you mean" suggestions by substring (client.js:169-187). Channel matching is an exact lowercase match on the raw name with no normalization (client.js:189-201).
- Export files are `~/.discord-cli/exports/<normalized guild name>/<normalized channel name>.json` with envelope `{guild:{id,name}, channel:{id,name,type}, dateRange:{after,before}, messages, messageCount}` (export.js:64-73, store.js:5, 100-106). Messages are raw API objects sorted by timestamp ascending.
- `.meta.json` in the same directory holds `{channels:{<channelId>:{lastMessageId,lastExport,messageCount}}, lastExport}` (store.js:26-42, live file). Incremental export fetches `after=lastMessageId`, drops ids already on disk, concatenates, and sorts ascending (export.js:24-61).
- DiscordChatExporter JSON uses a different message shape (`author.name`, `author.nickname`, `timestampEdited`, `isPinned`, string `type`, local-offset timestamps). The CLI bridges only the author field, by reading `author.nickname || author.name || author.username` in local search (search.js:161, 173).
- Automating a user account violates Discord's terms of service per DiscordChatExporter's own docs (Token-and-IDs.md:10). The unofficial docs add that 10,000 invalid requests (401, 403, 429) in 10 minutes cause a 24 hour ban (rate-limits.md:110).

## 1. Authentication and header set

### What the Node CLI sends

The token comes from `DISCORD_TOKEN` and the process exits with code 1 if it is missing (client.js:23-30). It is sent as-is in `Authorization`, with no `Bot ` or `Bearer ` prefix (client.js:34). The unofficial docs describe this form as the user token form: "authentication is performed with the `Authorization` HTTP header in the format `Authorization: TOKEN_TYPE? TOKEN`" with the bare token example first (reference.md:97-101).

Every request sends exactly these headers (client.js:32-45):

| Header | Exact value | Source line | Documented by the unofficial docs? |
|---|---|---|---|
| `Authorization` | the raw token | client.js:34 | Yes (reference.md:97-101) |
| `User-Agent` | `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36` | client.js:3, 35 | Yes. "Clients using the HTTP API should provide a valid User Agent ... For user accounts, see the Client Properties section" (reference.md:222) |
| `X-Super-Properties` | base64 of the JSON below | client.js:5-21, 36 | Yes (reference.md:296) |
| `X-Discord-Locale` | `en-US` | client.js:37 | Yes. "User locale is determined by looking at the `X-Discord-Locale` header, then the `Accept-Language` header if not present, then lastly the user settings locale" (reference.md:1029) |
| `X-Discord-Timezone` | the process time zone from `Intl.DateTimeFormat().resolvedOptions().timeZone` | client.js:38 | No. I grepped reference.md for "timezone" and found only a note about timestamp rendering (reference.md:707) |
| `X-Debug-Options` | `bugReporterEnabled` | client.js:39 | Partly. The header is documented, but only `canary` and `trace` are listed as values with an effect (reference.md:242-247). `bugReporterEnabled` is not listed |
| `Sec-Ch-Ua` | `"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"` | client.js:40 | No. Not mentioned in reference.md |
| `Sec-Ch-Ua-Mobile` | `?0` | client.js:41 | No |
| `Sec-Ch-Ua-Platform` | `"macOS"` | client.js:42 | No |
| `Referer` | `https://discord.com/channels/@me` | client.js:43 | No |

The `X-Super-Properties` value is `Buffer.from(JSON.stringify(obj)).toString('base64')` where `obj` is, in this key order (client.js:5-21):

```json
{
  "os": "Mac OS X",
  "browser": "Chrome",
  "device": "",
  "system_locale": "en-US",
  "has_client_mods": false,
  "browser_user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
  "browser_version": "146.0.0.0",
  "os_version": "10.15.7",
  "referrer": "",
  "referring_domain": "",
  "referrer_current": "",
  "referring_domain_current": "",
  "release_channel": "stable",
  "client_build_number": 523497,
  "client_event_source": null
}
```

The value is computed once at module load and does not change between requests (client.js:5). The `browser_user_agent` field equals the `User-Agent` header, which matches the docs' note that "If specified, this value should match the `User-Agent` header sent by the client" (reference.md:362).

### What the unofficial docs say about client properties

The docs define the header as follows: "Client properties, or 'super properties', contain tracking information about the current client, used for analytics and A/B testing purposes. These properties are sent when identifying with the Gateway and are included with every outgoing HTTP request using the `X-Super-Properties` header as a base64-encoded JSON object" (reference.md:296).

Two more statements bear on whether the header is required:

- "When these properties are not provided, Discord will attempt to parse some of them from the `User-Agent` header of requests" (reference.md:298).
- "Due to the nature of client properties, the structure of this object is not well-defined, and no field is truly required" (reference.md:302).
- Footnote 1 on `os`, `client_build_number`, and `release_channel`: "These properties are used to gate experimental features and may be required for certain endpoints" (reference.md:306, 314, 324, 360).

The docs' own web example carries the same field set as the CLI plus `client_launch_id`, `launch_signature`, `client_heartbeat_session_id`, `search_engine_current`, and `mp_keyword_current` (reference.md:536-561). The CLI omits those five.

### What DiscordChatExporter sends

In `DiscordClient.cs` the only header set explicitly is `Authorization`, using `Bot {token}` for bot tokens and the bare token otherwise (DiscordClient.cs:39-44). The comment explains the use of `TryAddWithoutValidation`: "Don't validate because the token can have special characters" (DiscordClient.cs:39-40). The shared `Http.Client` is defined in another file that I did not download, so I cannot say whether it sets a `User-Agent`. The project README states "Authentication via either a user or a bot token" (dce-readme.md:45), so a user token works with this smaller header set.

### Which headers are required

Inference: `Authorization` is the only header with evidence of being required, because DiscordChatExporter works with just that header in its client class. The `User-Agent` and `X-Super-Properties` headers are the next most likely to matter, because the docs say Discord parses client properties from the user agent when the header is missing (reference.md:298) and that some properties "may be required for certain endpoints" (reference.md:360). `X-Discord-Locale` has a documented effect on locale only (reference.md:1029). `X-Discord-Timezone`, `X-Debug-Options: bugReporterEnabled`, the three `Sec-Ch-Ua*` headers, and `Referer` have no documented effect in any source I read, so I treat them as imitation of the browser rather than as requirements. None of this was tested against the live API in this research.

## 2. Endpoints and query parameters by command

All paths are relative to `https://discord.com/api/v10` (client.js:1). The `request` helper appends every non-null option as a query string parameter with `URLSearchParams.set`, so booleans become the strings `true` and `false` and numbers become decimal strings (client.js:49-52). The docs accept `true`, `True`, or `1` for boolean query strings (reference.md:238). Every call is a GET; there is no request body anywhere in the CLI (client.js:55-57).

### Endpoint table

| Command | Endpoint | Query parameters sent | Source | Docs |
|---|---|---|---|---|
| guilds, export, and every name lookup | `GET /users/@me/guilds` | `with_counts=true`, `limit=200` | client.js:86-88 | Params `before`, `after`, `limit` (1 to 200, default 200), `with_counts` (raw-guild.mdx:1944-1951). "This endpoint returns 200 guilds by default, which is the maximum number of guilds a non-bot user can join" (raw-guild.mdx:1940) |
| info | `GET /guilds/{guildId}` | `with_counts=true` | client.js:90-92 | `with_counts` "Whether to include approximate member and presence counts (default false)" (raw-guild.mdx:2029-2039) |
| channels, info, export, channel name lookup | `GET /guilds/{guildId}/channels` | none | client.js:94-96 | |
| channels `--threads`, info, export `--threads` | `GET /guilds/{guildId}/threads/active` | none | client.js:98-101 | "This endpoint is not usable by user accounts" (raw-channel.mdx:1245-1255). See section 3 |
| (defined, never called by a command) | `GET /channels/{channelId}/threads/archived/{public|private}` | `limit=100`, `before=<archive_timestamp>` | client.js:103-116 | `before` is an "ISO8601 timestamp", `limit` is 2 to 100 default 50, response has `threads`, `members`, `has_more` (raw-channel.mdx:1289-1308) |
| messages | `GET /channels/{channelId}/messages` | `limit` (caller value or 100), optional `before`, optional `after` | client.js:120-125 | Params `around`, `before`, `after` (snowflakes) and `limit` "1-100, default 50" (message.md:1264-1271, route at raw-message.mdx:1372) |
| export, search `--dm` | `GET /channels/{channelId}/messages` in a loop | `limit=100`, optional `before`, optional `after` | client.js:127-144 | same |
| search (guild) | `GET /guilds/{guildId}/messages/search` | `content`, `author_id`, `channel_id`, `has`, `limit` (capped at 25), `offset`, `sort_by`, `sort_order` | client.js:148-159 | See below |
| dms, messages `--dm`, search `--dm` | `GET /users/@me/channels` | none | client.js:163-165 | "Returns a list of active private channel objects the user is participating in" (raw-channel.mdx:664-668) |

Nothing in the CLI calls `/users/@me`, so the CLI never checks the token before its first real request. Compare DiscordChatExporter, which probes `users/@me` first (DiscordClient.cs:101-121).

### Message listing and pagination

`getMessages` is used by the `messages` command with `limit = min(--limit, 100)` (messages.js:93), plus `--before` and `--after` passed straight through as message ids (messages.js:94-97). The API returns newest first, so the command reverses the array before printing (messages.js:109-110).

`getAllMessages` is the full-history loop used by export and by DM search (client.js:127-144). Each iteration:

1. Sends `limit=100`, plus `before` if set, plus `after` if set (client.js:133-136).
2. Stops if the page is empty (client.js:137).
3. Appends the page and calls the progress callback with `(pageSize, runningTotal)` (client.js:138-139).
4. Stops if the page has fewer than 100 items (client.js:140).
5. Sets `before` to the id of the last item in the page, which is the oldest item because the API returns newest first (client.js:141, export.js:60).

The loop keeps `after` fixed at its initial value while it moves `before` (client.js:130, 135). So an incremental export that starts with `after=lastMessageId` will, on its second page, send both `after` and `before`. DiscordChatExporter's comment says "Discord API doesn't allow us to provide both 'after' and 'before' parameters at the same time" (DiscordClient.cs:682-683). The unofficial docs list `around`, `before`, and `after` as separate optional parameters and do not say whether they can be combined (message.md:1264-1271). These two sources disagree, or at least the docs are silent. Inference: if the API rejects the combination or ignores one parameter, an incremental export with more than 100 new messages either fails on the second page or returns the wrong slice. I did not test this. A Go port should paginate forward with `after` only, the way DiscordChatExporter does (DiscordClient.cs:707-714, 783-785), and set the next boundary to the newest id in each page.

DiscordChatExporter also notes a second constraint that shapes its design: "Use the regular message listing endpoint with the 'around' parameter instead of the dedicated single-message endpoint, because the latter is not accessible to user tokens" (DiscordClient.cs:601-602). The Node CLI never fetches a single message, so it is not affected, but a Go port that adds that feature must use `around` with `limit=1` (DiscordClient.cs:603-607).

### Guild search

The `search` command without `--local` or `--dm` calls `liveSearchGuild` (search.js:245-250). It always sends `sort_by=timestamp` and `sort_order=desc` (search.js:34), and adds (search.js:36-43):

- `content` from `--query` or the first bare argument (search.js:15, 24, 36).
- `has` from `--has` (search.js:37).
- `limit` from `--limit`, capped at 25 (search.js:38, again at client.js:154).
- `offset` from `--offset` (search.js:39).
- `channel_id` resolved from `--channel` (search.js:41-43).

The flags `--author`, `--after`, and `--before` are parsed (search.js:16-18) but never sent for guild search. The client helper supports `author_id` (client.js:151) but `liveSearchGuild` never sets it. The help text says `--author` is "local only" (search.js:225).

The docs for `GET /guilds/{guild.id}/messages/search` (raw-message.mdx:1423-1427) list the relevant parameters as (message.md:1301-1333):

| Parameter | Docs description |
|---|---|
| `limit` | "Max number of messages to return (1-25, default 25)" |
| `offset` | "Number to offset the returned messages by (max 9975)" |
| `max_id`, `min_id` | "Get messages before/after this message ID"; footnote 6: "When sorting by `timestamp`, these parameters may be used for pagination instead of `offset`. This allows search to paginate through more than 10,000 results" (message.md:1343) |
| `content` | "Filter messages by content (max 1024 characters)" |
| `channel_id` | "array[snowflake]", "Filter messages by these channels (max 500)" |
| `author_id` | "array[snowflake]", "Filter messages by these authors (max 100)" |
| `has` | "array[string]", values `image`, `sound`, `video`, `file`, `sticker`, `embed`, `link`, `poll`, `snapshot`, each negatable with a `-` prefix (message.md:1357-1371) |
| `sort_by` | `timestamp` (default) or `relevance` (message.md:1387-1392) |
| `sort_order` | "`asc` or `desc`, default `desc`" |

The CLI sends `has` and `channel_id` as single values where the docs say array. The CLI help lists `attachment` as a `--has` value (search.js:229), which is not in the documented list. The docs say the search endpoint "Requires the `READ_MESSAGE_HISTORY` permission" and returns messages "without the `reactions` key" (message.md:1299), which is why `reactions` is always empty in search output even though search.js:55 counts it.

The response shape the CLI reads (search.js:46, 59):

```json
{
  "analytics_id": "...",
  "doing_deep_historical_index": false,
  "total_results": 1,
  "messages": [ [ { "id": "...", "channel_id": "...", "author": { "username": "..." }, "content": "...", "timestamp": "...", "attachments": [], "hit": true } ] ]
}
```

The docs confirm `messages` is "array[array[message object]]" and explain: "The nested array was used to provide surrounding context to search results. However, surrounding context is no longer returned" (message.md:1394-1407). The CLI flattens with `.flat()` (search.js:46). The docs also list optional `channels`, `threads`, and `members` keys that the CLI ignores (message.md:1403-1405).

Two warnings from the docs that the CLI does not handle:

- "If the entity you are searching is not yet indexed, the endpoint will return a 202 accepted response. The response body will not contain any search results" (raw-message.mdx:1435-1437). The CLI treats any 2xx as success and would read `messages` as undefined, then print "No results found." (client.js:67, search.js:46, 63-66).
- "For applications, this endpoint is restricted according to whether the `MESSAGE_CONTENT` Privileged Intent is enabled for the application" (raw-message.mdx:1429-1432). This applies to bots only.

### DM search

There is no DM search endpoint used by the CLI. The comment says "No DM search endpoint — paginate and filter locally" (search.js:79). It lists DMs, matches the recipient, pulls the whole history with `getAllMessages`, and filters in memory on content terms, `--after`, and `--before` (search.js:80-101). The skill file records the cost: "search --dm paginates all messages and filters locally, which can be slow for long DM histories" (SKILL.md:182). The docs do document `GET /channels/{channel.id}/messages/search` for private channels (raw-message.mdx:1618, message.md:1454-1456), which the CLI does not use.

### Local search

`--local` reads export files instead of the API. It gathers files for one guild (`--guild`) or every export directory (`--all`) (search.js:133-142), then filters each file's `messages` array on content terms, author, and date bounds (search.js:154-167). Results sort newest first and are cut to `--limit` (search.js:180-181).

### Response fields each command reads

- guilds: `id`, `name`, `approximate_member_count`, `owner` (guilds.js:21-26, 31-35). `approximate_member_count` is "Only included when fetched from the List User Guilds endpoint with `with_counts` set to `true`" (guild.md:518).
- channels: `id`, `name`, `type`, `parent_id`, `position` (channels.js:49-55, 60-76). Type labels are mapped from the numeric type at channels.js:4-8.
- dms: `id`, `type` (1 or 3 only), `name`, `recipients[].username`, `last_message_id` (dms.js:20-29).
- messages: `author.global_name`, `author.username`, `timestamp`, `content`, `embeds[].title|description|url`, `attachments[].filename|url`, `reactions[].emoji.name|count` (messages.js:20-43).
- info: `guild.id`, `guild.name`, `approximate_member_count`, `approximate_presence_count`, channel type counts, thread count, and local meta (info.js:42-62).
- export: whole message objects, unmodified (export.js:71).

## 3. User versus bot endpoint branches

### How DiscordChatExporter resolves the token kind

`ResolveTokenKindAsync` caches the result and tries `users/@me` first as a user token, then as a bot token (DiscordClient.cs:94-122). Any status other than 401 counts as success for that kind (DiscordClient.cs:108-109, 118-119). If both return 401 it throws "Authentication token is invalid." (DiscordClient.cs:121). The kind selects the `Authorization` prefix (DiscordClient.cs:41-44).

### Threads

The branch is explicit (DiscordClient.cs:451-452 and 509-510):

- Comment: "User accounts can only fetch threads using the search endpoint" (DiscordClient.cs:451). For each non-category, non-voice, non-empty channel it calls `channels/{channel.Id}/threads/search` with `sort_by=last_message_time`, `sort_order=desc`, `archived=false` or `true`, and `offset=<count so far>` (DiscordClient.cs:465-471). It stops when `has_more` is false (DiscordClient.cs:503-504). The comment notes the response "Can be null on channels that the user cannot access or channels without threads" (DiscordClient.cs:473).
- Comment: "Bot accounts can only fetch threads using the threads endpoint" (DiscordClient.cs:509). It calls `guilds/{guildId}/threads/active` (DiscordClient.cs:521-523), then, if archived threads are wanted, `channels/{channel.Id}/threads/archived/{public|private}` paged by `before=<archive_timestamp of the last thread>` (DiscordClient.cs:559-580). The comment warns "This endpoint parameter expects an ISO8601 timestamp, not a snowflake" (DiscordClient.cs:551).

The unofficial docs agree. Under `GET /guilds/{guild.id}/threads/active` there is a warning box: "This endpoint is not usable by user accounts" (raw-channel.mdx:1245-1255). `GET /channels/{channel.id}/threads/search` is documented with `archived`, `sort_by` (values `last_message_time` default, `archive_time`, `relevance`, `creation_time`), `sort_order`, `limit` "1-25, default 25", `offset` "max 9975", and `max_id`/`min_id` for pagination when sorting by `creation_time` (channel.md:1080-1111). Its response has `threads`, `members`, `has_more`, `total_results`, and `first_messages` (channel.md:1113-1123).

### What the Node CLI does instead

The CLI calls the bot-only endpoint `GET /guilds/{guildId}/threads/active` and reads `data.threads` (client.js:98-101). Every caller wraps it in `try { ... } catch { /* bot-only on some servers */ }` (channels.js:43, info.js:37, export.js:151). So on a user token `--threads` produces no threads and no error. The skill file describes the outcome as "Active thread listing requires bot auth on some servers. The CLI handles this gracefully (falls back silently)" (SKILL.md:184). The docs say it is not usable by user accounts on any server, not just some (raw-channel.mdx:1253). The archived-thread helper in client.js:103-116 uses the correct `before=<archive_timestamp>` mechanic but no command calls it.

A Go port that wants threads on a user token must use `/channels/{id}/threads/search` per channel, as DiscordChatExporter does.

### MESSAGE_CONTENT intent check, bots only

`EnsureMessageContentIntentAsync` returns immediately unless the token kind is Bot (DiscordClient.cs:199-200). For bots it fetches `applications/@me` and throws "Provided bot account is missing the MESSAGE_CONTENT privileged intent." if the flag is off (DiscordClient.cs:202-209). It is called only when a whole page of messages is empty (DiscordClient.cs:729-733), with the comment "If all messages are empty, make sure that it's not because the bot account doesn't have the MESSAGE_CONTENT intent enabled." The user guide says the same for bot setup: "If this option is not enabled, the exported files will be empty" (Token-and-IDs.md:284-286). None of this applies to a user token, so the Go port can skip it for the CLI's use case.

## 4. Rate limit handling

### Node CLI

The request loop runs at most two attempts (client.js:54). On a 429 it parses the JSON body, waits `retry_after` seconds (or 1 second if the field is missing), prints "Rate limited, waiting Ns..." to stderr, and tries again (client.js:59-65). A second 429 falls out of the loop and throws "Rate limited after retry" (client.js:81). Any other non-2xx status throws `Discord API <status>: <body>` without retry (client.js:67-70).

After a successful response, the code reads `x-ratelimit-remaining` and `x-ratelimit-reset-after`, and if remaining is the string `"0"` and reset-after is present it sleeps `parseFloat(resetAfter)` seconds before returning (client.js:72-77). The comment is "Respect rate limits proactively" (client.js:72). There is no buffer and no cap.

### DiscordChatExporter

The same two signals are read, but with three differences (DiscordClient.cs:52-87):

- The proactive sleep is optional. The comment explains: "Discord has advisory rate limits (communicated via response headers), but they are typically way stricter than the actual rate limits enforced by the server. The user may choose to ignore the advisory rate limits and only retry on hard rate limits, if they want to prioritize speed over compliance (and safety of their account/bot)" (DiscordClient.cs:52-56). The check is `rateLimitPreference.IsRespectedFor(tokenKind)` (DiscordClient.cs:57).
- It waits when remaining is less than or equal to zero. The comment: "If this was the last request available before hitting the rate limit, wait out the reset time so that future requests can succeed. This may add an unnecessary delay in case the user doesn't intend to make any more requests, but implementing a smarter solution would require properly keeping track of Discord's global/per-route/per-resource rate limits and that's just way too much effort" (DiscordClient.cs:68-74).
- It adds one second and caps at 60 seconds. The comments: "Adding a small buffer to the reset time reduces the chance of getting rate limited again, because it allows for more requests to be released" and "Sometimes Discord returns an absurdly high value for the reset time, which is not actually enforced by the server. So we cap it at a reasonable value" (DiscordClient.cs:78-83). The code is `(resetAfterDelay.Value + TimeSpan.FromSeconds(1)).Clamp(TimeSpan.Zero, TimeSpan.FromSeconds(60))` (DiscordClient.cs:80-83).

Handling of actual 429 responses is not in this file. It lives in `Http.ResponseResiliencePipeline` (DiscordClient.cs:34), which I did not download, so I cannot quote its retry policy.

### What the docs say about the headers

Both the official and unofficial docs define the headers the same way. `X-RateLimit-Remaining` is "The number of remaining requests that can be made" and `X-RateLimit-Reset-After` is "Total time (in seconds) of when the current rate limit bucket will reset. Can have decimals to match previous millisecond ratelimit precision" (official-rate-limits.md:12-14, rate-limits.md:25-27). On 429, "Your application should rely on the `Retry-After` header or `retry_after` field to determine when to retry the request" (official-rate-limits.md:21). The 429 body is `{message, retry_after (float, seconds), global (boolean), code?}` (rate-limits.md:35-42).

One statement in the unofficial docs cuts against the Node CLI's proactive sleep: "For most API requests made with bot or OAuth2 authorization, Discord returns optional HTTP response headers containing the rate limit encountered during your request. User authorization *usually* only returns the **Retry-After**, **X-RateLimit-Global**, and **X-RateLimit-Scope** headers" (rate-limits.md:11). Inference: if that is accurate, the `x-ratelimit-remaining` check at client.js:73-77 rarely fires on a user token, and the 429 path is the one that does the work. DiscordChatExporter's comment that the advisory headers are "way stricter than the actual rate limits" (DiscordClient.cs:53) suggests the headers are at least sometimes present for user tokens. I did not measure this.

Two limits a Go port should keep in mind:

- "All users can make up to 50 requests per second to our API" (rate-limits.md:100).
- "IP addresses that make too many invalid HTTP requests are automatically and temporarily restricted from accessing the Discord API. Currently, this limit is **10,000 per 10 minutes** and leads to a **24 hour ban**. An invalid request is one that results in **401**, **403**, or **429** statuses" (rate-limits.md:110). The official page says the same minus the "24 hour ban" phrase (official-rate-limits.md:39). The export command runs four channels at once (export.js:6, 91-109), which multiplies the chance of 429s.

## 5. Guild and channel name resolution

### The normalization rule

`normalizeName` in store.js:8-10 is:

```js
name.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '')
```

The same expression appears inline for guild matching (client.js:175). In words: lowercase the whole string, replace each run of whitespace with one hyphen, then delete every character that is not a lowercase ASCII letter, a digit, or a hyphen. Emoji, punctuation, and non-ASCII letters are deleted, not transliterated. Leading or trailing hyphens are kept. Real outputs on disk: "Cooey COE" became the directory `cooey-coe`, and the channel "🔮general" became `general.json` (general.json head, lines 3-8 of that file). A file named `sprs-.json` exists in the same directory, which shows a trailing hyphen surviving from a name that ended in whitespace plus a stripped character.

### Guild matching

`resolveGuildId` (client.js:169-187):

1. If the input is all digits, return it as the id (client.js:170).
2. Fetch all guilds (client.js:171).
3. Lowercase the input. A guild matches if its lowercase name equals the input, or its normalized name equals the input (client.js:172-176). The input itself is not normalized, so `cooey coe` matches by the first rule and `cooey-coe` by the second, but `Cooey_COE` matches neither.
4. If nothing matches, collect guilds whose lowercase name contains the input as a substring. If any exist, throw `Guild not found: "<input>". Did you mean: <names joined by ", ">?`. Otherwise throw `Guild not found: "<input>". Available: <all names>` (client.js:177-185).

### Channel matching

`resolveChannelId` (client.js:189-201):

1. All-digit input is returned as the id (client.js:190).
2. Fetch the guild's channels (client.js:191).
3. A channel matches only if its lowercase name equals the lowercase input (client.js:192-193). There is no normalization on either side, so the emoji prefix must be typed, e.g. `📰news`. The help text confirms this with the example `--channel 📰news` (messages.js:67).
4. If nothing matches, throw `Channel not found: "<input>". Available: <names>` where the list is filtered to channel types 0, 5, and 15 (client.js:194-198). There is no "Did you mean" for channels.

The skill file says "Channel names work the same way, including emoji prefixes" as guild names (SKILL.md:46), which overstates it. Guild names accept the hyphenated form; channel names do not.

### The channel type filter [0, 5, 15]

The set `[0, 5, 15]` appears in three places: the "Available" list on a failed channel lookup (client.js:196), the text channel count in `info` (info.js:42), and the export channel filter with the comment "Filter to text-based channels (text, announcement, forum)" (export.js:142-143). The docs define them as 0 `GUILD_TEXT`, 5 `GUILD_NEWS` ("Almost identical to `GUILD_TEXT`, a channel that users can follow and crosspost"), and 15 `GUILD_FORUM` ("A channel that can only contain threads") (channel.md:135, 140, 150). Type 16 `GUILD_MEDIA` (channel.md:151) is labelled in channels.js:7 but excluded from export.

### DM matching

The `dms` command keeps only types 1 and 3 (dms.js:21), which the docs name `DM` and `GROUP_DM` (channel.md:136, 138). It labels a type 3 channel with its `name` or, if absent, the recipients' usernames joined by ", " (dms.js:25-27).

`messages --dm` picks the first DM channel where any recipient's `username` or `global_name` contains the lowercase input as a substring (messages.js:76-81). `search --dm` uses only `username` (search.js:81-84). Both throw `DM not found for: "<input>"` on no match (messages.js:83, search.js:85). Because it is a first-match substring search over the DM list in API order, a short input such as `al` can match an unintended person.

## 6. Export envelope and .meta.json

### Directories and file names

- New exports: `~/.discord-cli/exports/<normalized guild name>/` (store.js:5, 12-14). The guild name comes from the `/users/@me/guilds` response, falling back to the `--guild` argument if the id is not found (export.js:134-137).
- Legacy exports: `~/DiscordChatExporter.Cli.osx-arm64/exports/` (store.js:6). This directory is only read, never written (store.js:46-91).
- File name: `<normalized channel name>.json` (store.js:103). The write is `JSON.stringify(data, null, 2)`, so two-space pretty printing (store.js:104).

Because the file name is the normalized channel name and not the channel id, two channels whose names differ only in emoji or case, e.g. "general" and "🔮general", would overwrite each other. The DiscordChatExporter naming on disk includes the id, e.g. `Cooey COE - access-control - 3.1.8 [<channel-id>].json` (directory listing of the legacy exports).

### The envelope

`exportChannel` writes this object (export.js:64-73):

```json
{
  "guild":   { "id": "<guildId>", "name": "<guild name as returned by the API>" },
  "channel": { "id": "<channel.id>", "name": "<channel.name, raw, with emoji>", "type": <numeric channel type> },
  "dateRange": {
    "after":  "<timestamp of the oldest message, or null>",
    "before": "<timestamp of the newest message, or null>"
  },
  "messages": [ /* raw API message objects, oldest first */ ],
  "messageCount": <messages.length>
}
```

A live file confirms the shape: `general.json` opens with `guild.id` `<guild-id>`, `guild.name` `Cooey COE`, `channel.name` `🔮general`, `channel.type` `0`, and `dateRange.after` `2019-05-19T20:10:55.344000+00:00` (first 16 lines of `~/.discord-cli/exports/cooey-coe/general.json`). The top-level keys are exactly `channel, dateRange, guild, messageCount, messages` (jq `keys` on announcements.json). The comment above the object says "Write in DiscordChatExporter-compatible format" (export.js:63), but `dateRange` means something different in the two tools. In the Node file it is the span of the data. In the DiscordChatExporter file it is the requested filter and is `null`/`null` when no range was requested (legacy 3.1.8 file, lines 13-16).

### .meta.json

Path: `<export dir>/.meta.json` (store.js:22-24). Default when missing or unreadable: `{ channels: {}, lastExport: null }` (store.js:26-33). Each update merges the new fields into the channel's entry and sets the top-level `lastExport` to the current time (store.js:35-42). Shape, with values from the live file:

```json
{
  "channels": {
    "<channel-id>": {
      "lastMessageId": "<message-id>",
      "lastExport": "2026-04-07T06:32:31.274Z",
      "messageCount": 90
    }
  },
  "lastExport": "2026-09-02T07:35:31.598Z"
}
```

(`~/.discord-cli/exports/cooey-coe/.meta.json`, first entry and last line). Keys are channel ids as strings. `lastMessageId` is the id of the newest message in the merged array (export.js:78-84). `lastExport` values are JavaScript `toISOString()` output with millisecond precision and a `Z` suffix (store.js:38, export.js:82).

### Incremental merge

Per channel, `exportChannel` does the following (export.js:20-89):

1. Read the meta file and look up the channel id (export.js:21-22).
2. Unless `--full` is set, if `lastMessageId` exists, set `after` to it (export.js:24-27). This is what makes the run incremental (export.js:29).
3. Fetch with `getAllMessages` (export.js:32-35). See the pagination caveat in section 2.
4. If incremental and nothing came back, print "up to date" and return without writing (export.js:37-40).
5. If incremental, find the existing file by taking the first export file whose path contains the normalized channel name, read it, build a set of existing ids, keep only fetched messages whose id is not in the set, concatenate existing then new, and sort by `timestamp` ascending (export.js:43-58). If reading fails, the fetched messages are used alone (export.js:55-57).
6. Sort ascending again in all cases (export.js:61).
7. Write the envelope and update the meta entry with `lastMessageId`, `lastExport`, and `messageCount` (export.js:75-85).

Inference on step 5: the file lookup is `f.includes(store.normalizeName(channel.name))` (export.js:48), a substring test. In the `cooey-coe` directory the files `general.json`, `cmmc-general.json`, and `general-cmmc-scoping.json` all contain `general`, so which file the channel "🔮general" merges into depends on directory read order. A Go port should look up by channel id from `.meta.json` or by exact file name, not by substring.

Inference on concurrency: `runPool` runs four `exportChannel` tasks at once (export.js:6, 91-109, 157), and each one reads and rewrites `.meta.json` (store.js:35-42). Two channels finishing at the same time can lose one update. A Go port should serialize meta writes.

### The exported message objects

Messages are stored exactly as the API returns them. The first message in `announcements.json` has the keys `attachments, author, channel_id, components, content, edited_timestamp, embeds, flags, id, mention_everyone, mention_roles, mentions, pinned, timestamp, tts, type`, and its `author` has `accent_color, avatar, avatar_decoration_data, banner, banner_color, clan, collectibles, discriminator, display_name_styles, flags, global_name, id, primary_guild, public_flags, username` (jq on announcements.json). Messages that carry `reactions`, `message_reference`, `sticker_items`, or `thread` fields would include them because nothing is stripped (export.js:71).

## 7. DiscordChatExporter export dialect differences

Both files were inspected with `head` and `jq`, not read in full.

### Side by side

| Field | Node CLI export (`~/.discord-cli/exports/cooey-coe/general.json`) | DiscordChatExporter export (`Cooey COE - access-control - 3.1.8 [<channel-id>].json`) |
|---|---|---|
| top-level keys | `channel, dateRange, guild, messageCount, messages` | `channel, dateRange, exportedAt, guild, messageCount, messages` |
| `guild` | `{id, name}` | `{id, name, iconUrl}` (lines 2-6) |
| `channel` | `{id, name, type}` with numeric `type` (0) | `{id, type, categoryId, category, name, topic}` with string `type` (`GuildPublicThread`) (lines 7-14) |
| `dateRange` | span of the stored messages | requested filter; `{after: null, before: null}` here (lines 15-18) |
| message keys | `attachments, author, channel_id, components, content, edited_timestamp, embeds, flags, id, mention_everyone, mention_roles, mentions, pinned, timestamp, tts, type` | `attachments, author, callEndedTimestamp, content, embeds, id, inlineEmojis, isPinned, mentions, reactions, stickers, timestamp, timestampEdited, type` |
| message `type` | integer, e.g. `7` | string, e.g. `Default` (line 23) |
| timestamp | `2019-05-19T20:10:55.344000+00:00` (UTC, microseconds) | `2022-09-15T12:43:11.959-07:00` (local offset, milliseconds) (line 24) |
| edit time | `edited_timestamp` | `timestampEdited` (line 25) |
| pinned | `pinned` | `isPinned` (line 27) |
| `author` keys | `accent_color, avatar, avatar_decoration_data, banner, banner_color, clan, collectibles, discriminator, display_name_styles, flags, global_name, id, primary_guild, public_flags, username` | `avatarUrl, color, discriminator, id, isBot, name, nickname, roles` |
| author display name | `global_name` | `nickname` (guild nickname, `<nickname>`) and `name` (`<handle>`) (lines 30-33) |
| author handle | `username` | `name` |
| roles | not present | `roles[]` with `id, name, color, position` (lines 36-42) |
| `channel_id` on message | present | absent |

The DiscordChatExporter `author.name` holds what the Node file calls `username`, and `author.nickname` holds the guild nickname, which the raw API message object does not carry at all (raw API author objects have no nickname; see the key list above).

### How search.js bridges the two

Local search reads the author name as `msg.author?.nickname || msg.author?.name || msg.author?.username || ''` both for the `--author` filter (search.js:161) and for the displayed author (search.js:173). So on a DiscordChatExporter file it shows the nickname, and on a Node file it falls through to `username`. It never reads `global_name`, so on Node files the display name is the handle, not the display name. Every other field the local search touches (`content`, `timestamp`, `channel.name`, `messages`) has the same key in both dialects (search.js:153-175). Nothing bridges `type`, `pinned`/`isPinned`, or `edited_timestamp`/`timestampEdited`.

The `info` command counts files from both directories through `getExportFiles` (info.js:40, store.js:63-91), and `search --local --all` walks both (search.js:136-141, store.js:46-61). A Go port that keeps legacy exports searchable needs to keep reading both shapes.

## 8. Terms-of-service exposure

DiscordChatExporter's token guide states, under "How to get a user token": "**Caution:** Automating user accounts violates Discord's terms of service and may result in account termination. Use at your own risk." The linked page is `https://support.discord.com/hc/en-us/articles/115002192352-Automated-user-accounts-self-bots-` (Token-and-IDs.md:8-10).

The same file warns about the token itself: "**Do not share your token!** A token gives full access to an account. To reset a user token, change your account password" (Token-and-IDs.md:3-5).

The CLI is squarely in this category. It uses a user token (client.js:23-34), imitates the web client's headers (client.js:35-43), and runs up to four export loops at once (export.js:6). The unofficial docs add the operational risk that too many 401, 403, or 429 responses from one IP cause a 24 hour ban (rate-limits.md:110). The Go port inherits all of this and should say so in its own documentation.

## Things I could not source

- Whether the `Sec-Ch-Ua*`, `Referer`, `X-Discord-Timezone`, and `X-Debug-Options: bugReporterEnabled` headers have any server-side effect. No page I read mentions them.
- Whether `before` and `after` can be combined on `/channels/{id}/messages`. DiscordChatExporter says no; the unofficial docs are silent.
- DiscordChatExporter's 429 retry policy and default `User-Agent`. Both live in `Http.cs`, which was not downloaded.
- Whether user tokens receive `X-RateLimit-Remaining` on ordinary responses. The unofficial docs say "usually" not; DiscordChatExporter's comments imply sometimes yes.
- The behavior of the substring file lookup in incremental export and the concurrent `.meta.json` writes. Both are read from the code, not observed.

## Sources

Local files (line numbers as printed by `cat -n`):

- `legacy/node/bin/discord.js`
- `legacy/node/src/client.js`
- `legacy/node/src/store.js`
- `legacy/node/src/formatter.js`
- `legacy/node/src/commands/guilds.js`
- `legacy/node/src/commands/channels.js`
- `legacy/node/src/commands/dms.js`
- `legacy/node/src/commands/messages.js`
- `legacy/node/src/commands/search.js`
- `legacy/node/src/commands/export.js`
- `legacy/node/src/commands/info.js`
- `~/.claude/skills/searching-discord/SKILL.md` (cited as SKILL.md)
- `~/.discord-cli/exports/cooey-coe/general.json`, `announcements.json`, `.meta.json` (read with `head -c` and `jq`)
- `~/DiscordChatExporter.Cli.osx-arm64/exports/cooey-coe/Cooey COE - access-control - 3.1.8 [<channel-id>].json` (read with `head -n 70` and `jq`)

Downloaded to the scratch directory `<scratch directory>/`:

- `DiscordClient.cs`, DiscordChatExporter `DiscordChatExporter.Core/Discord/DiscordClient.cs` (879 lines)
- `Token-and-IDs.md`, DiscordChatExporter `.docs/Token-and-IDs.md`
- `dce-readme.md`, DiscordChatExporter README
- `message.md` from https://docs.discord.food/resources/message (via `defuddle parse --md`)
- `channel.md` from https://docs.discord.food/resources/channel
- `guild.md` from https://docs.discord.food/resources/guild
- `reference.md` from https://docs.discord.food/reference
- `rate-limits.md` from https://docs.discord.food/topics/rate-limits
- `official-rate-limits.md` from https://discord.com/developers/docs/topics/rate-limits
- `raw-message.mdx` from https://raw.githubusercontent.com/discord-userdoccers/discord-userdoccers/master/pages/resources/message.mdx
- `raw-channel.mdx` from https://raw.githubusercontent.com/discord-userdoccers/discord-userdoccers/master/pages/resources/channel.mdx
- `raw-guild.mdx` from https://raw.githubusercontent.com/discord-userdoccers/discord-userdoccers/master/pages/resources/guild.mdx
- `RouteHeader.tsx` from https://raw.githubusercontent.com/discord-userdoccers/discord-userdoccers/master/components/RouteHeader.tsx (used only to confirm that `supportsBot` means "Supports bot users")

The `defuddle` conversions dropped some `###` headings and route lines, so endpoint paths are cited from the raw `.mdx` files and parameter tables from the `.md` files.
