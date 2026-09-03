const BASE = 'https://discord.com/api/v10';

const USER_AGENT = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36';

const SUPER_PROPERTIES = Buffer.from(JSON.stringify({
  os: 'Mac OS X',
  browser: 'Chrome',
  device: '',
  system_locale: 'en-US',
  has_client_mods: false,
  browser_user_agent: USER_AGENT,
  browser_version: '146.0.0.0',
  os_version: '10.15.7',
  referrer: '',
  referring_domain: '',
  referrer_current: '',
  referring_domain_current: '',
  release_channel: 'stable',
  client_build_number: 523497,
  client_event_source: null,
})).toString('base64');

function getToken() {
  const token = process.env.DISCORD_TOKEN;
  if (!token) {
    console.error('Error: DISCORD_TOKEN environment variable is not set.');
    process.exit(1);
  }
  return token;
}

function getHeaders(token) {
  return {
    Authorization: token,
    'User-Agent': USER_AGENT,
    'X-Super-Properties': SUPER_PROPERTIES,
    'X-Discord-Locale': 'en-US',
    'X-Discord-Timezone': Intl.DateTimeFormat().resolvedOptions().timeZone,
    'X-Debug-Options': 'bugReporterEnabled',
    'Sec-Ch-Ua': '"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"',
    'Sec-Ch-Ua-Mobile': '?0',
    'Sec-Ch-Ua-Platform': '"macOS"',
    Referer: 'https://discord.com/channels/@me',
  };
}

async function request(path, params = {}) {
  const token = getToken();
  const url = new URL(`${BASE}${path}`);
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null) url.searchParams.set(k, v);
  }

  for (let attempt = 0; attempt < 2; attempt++) {
    const res = await fetch(url.toString(), {
      headers: getHeaders(token),
    });

    if (res.status === 429) {
      const body = await res.json();
      const wait = (body.retry_after || 1) * 1000;
      process.stderr.write(`Rate limited, waiting ${(wait / 1000).toFixed(1)}s...\n`);
      await new Promise(r => setTimeout(r, wait));
      continue;
    }

    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Discord API ${res.status}: ${text}`);
    }

    // Respect rate limits proactively
    const remaining = res.headers.get('x-ratelimit-remaining');
    const resetAfter = res.headers.get('x-ratelimit-reset-after');
    if (remaining === '0' && resetAfter) {
      await new Promise(r => setTimeout(r, parseFloat(resetAfter) * 1000));
    }

    return res.json();
  }
  throw new Error('Rate limited after retry');
}

// --- Guild methods ---

export async function getGuilds() {
  return request('/users/@me/guilds', { with_counts: true, limit: 200 });
}

export async function getGuild(guildId) {
  return request(`/guilds/${guildId}`, { with_counts: true });
}

export async function getChannels(guildId) {
  return request(`/guilds/${guildId}/channels`);
}

export async function getActiveThreads(guildId) {
  const data = await request(`/guilds/${guildId}/threads/active`);
  return data.threads || [];
}

export async function getArchivedThreads(channelId, type = 'public') {
  const threads = [];
  let before = undefined;
  while (true) {
    const params = { limit: 100 };
    if (before) params.before = before;
    const data = await request(`/channels/${channelId}/threads/archived/${type}`, params);
    threads.push(...(data.threads || []));
    if (!data.has_more) break;
    const last = data.threads?.[data.threads.length - 1];
    before = last?.thread_metadata?.archive_timestamp;
  }
  return threads;
}

// --- Message methods ---

export async function getMessages(channelId, opts = {}) {
  const params = { limit: opts.limit || 100 };
  if (opts.before) params.before = opts.before;
  if (opts.after) params.after = opts.after;
  return request(`/channels/${channelId}/messages`, params);
}

export async function getAllMessages(channelId, opts = {}, onPage) {
  const messages = [];
  let before = opts.before || undefined;
  const after = opts.after || undefined;

  while (true) {
    const params = { limit: 100 };
    if (before) params.before = before;
    if (after) params.after = after;
    const page = await request(`/channels/${channelId}/messages`, params);
    if (page.length === 0) break;
    messages.push(...page);
    if (onPage) onPage(page.length, messages.length);
    if (page.length < 100) break;
    before = page[page.length - 1].id;
  }
  return messages;
}

// --- Search methods ---

export async function searchGuild(guildId, opts = {}) {
  const params = {};
  if (opts.content) params.content = opts.content;
  if (opts.author_id) params.author_id = opts.author_id;
  if (opts.channel_id) params.channel_id = opts.channel_id;
  if (opts.has) params.has = opts.has;
  if (opts.limit) params.limit = Math.min(opts.limit, 25);
  if (opts.offset) params.offset = opts.offset;
  if (opts.sort_by) params.sort_by = opts.sort_by;
  if (opts.sort_order) params.sort_order = opts.sort_order;
  return request(`/guilds/${guildId}/messages/search`, params);
}

// --- DM methods ---

export async function getDMs() {
  return request('/users/@me/channels');
}

// --- Utility ---

export async function resolveGuildId(nameOrId) {
  if (/^\d+$/.test(nameOrId)) return nameOrId;
  const guilds = await getGuilds();
  const lower = nameOrId.toLowerCase();
  const match = guilds.find(g =>
    g.name.toLowerCase() === lower ||
    g.name.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '') === lower
  );
  if (!match) {
    const suggestions = guilds
      .filter(g => g.name.toLowerCase().includes(lower))
      .map(g => g.name);
    const msg = suggestions.length
      ? `Did you mean: ${suggestions.join(', ')}?`
      : `Available: ${guilds.map(g => g.name).join(', ')}`;
    throw new Error(`Guild not found: "${nameOrId}". ${msg}`);
  }
  return match.id;
}

export async function resolveChannelId(guildId, nameOrId) {
  if (/^\d+$/.test(nameOrId)) return nameOrId;
  const channels = await getChannels(guildId);
  const lower = nameOrId.toLowerCase();
  const match = channels.find(c => c.name?.toLowerCase() === lower);
  if (!match) {
    const textChannels = channels
      .filter(c => [0, 5, 15].includes(c.type))
      .map(c => c.name);
    throw new Error(`Channel not found: "${nameOrId}". Available: ${textChannels.join(', ')}`);
  }
  return match.id;
}
