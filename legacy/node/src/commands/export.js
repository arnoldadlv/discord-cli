import * as client from '../client.js';
import * as store from '../store.js';
import { parseGlobalFlags, output } from '../formatter.js';

function parseArgs(args) {
  const opts = { concurrency: 4, threads: false, full: false };
  let i = 0;
  while (i < args.length) {
    const a = args[i];
    if (a === '--guild') opts.guild = args[++i];
    else if (a === '--channel') opts.channel = args[++i];
    else if (a === '--threads') opts.threads = true;
    else if (a === '--full') opts.full = true;
    else if (a === '--concurrency') opts.concurrency = parseInt(args[++i], 10);
    i++;
  }
  return opts;
}

async function exportChannel(guildId, channel, serverName, opts) {
  const meta = await store.getMeta(serverName);
  const channelMeta = meta.channels[channel.id] || {};

  const fetchOpts = {};
  if (!opts.full && channelMeta.lastMessageId) {
    fetchOpts.after = channelMeta.lastMessageId;
  }

  const isIncremental = !!fetchOpts.after;
  let totalMessages = 0;

  const messages = await client.getAllMessages(channel.id, fetchOpts, (pageSize, total) => {
    totalMessages = total;
    process.stderr.write(`\r  #${channel.name}: ${total} messages...`);
  });

  if (messages.length === 0 && isIncremental) {
    process.stderr.write(`\r  #${channel.name}: up to date\n`);
    return { channel: channel.name, messages: 0, skipped: true };
  }

  // If incremental, merge with existing export
  let allMessages = messages;
  if (isIncremental) {
    try {
      const existing = await store.readExport(
        (await store.getExportFiles(serverName))
          .find(f => f.includes(store.normalizeName(channel.name))) || ''
      );
      const existingMessages = existing.messages || [];
      const existingIds = new Set(existingMessages.map(m => m.id));
      const newMessages = messages.filter(m => !existingIds.has(m.id));
      allMessages = [...existingMessages, ...newMessages]
        .sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
    } catch {
      // No existing file, use fetched messages
    }
  }

  // Sort chronologically (API returns newest-first)
  allMessages.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

  // Write in DiscordChatExporter-compatible format
  const exportData = {
    guild: { id: guildId, name: serverName },
    channel: { id: channel.id, name: channel.name, type: channel.type },
    dateRange: {
      after: allMessages[0]?.timestamp || null,
      before: allMessages[allMessages.length - 1]?.timestamp || null,
    },
    messages: allMessages,
    messageCount: allMessages.length,
  };

  await store.writeExport(serverName, channel.name, exportData);

  // Update metadata for incremental
  const lastMsg = allMessages[allMessages.length - 1];
  if (lastMsg) {
    await store.setChannelMeta(serverName, channel.id, {
      lastMessageId: lastMsg.id,
      lastExport: new Date().toISOString(),
      messageCount: allMessages.length,
    });
  }

  process.stderr.write(`\r  #${channel.name}: ${allMessages.length} messages (${messages.length} new)\n`);
  return { channel: channel.name, messages: allMessages.length, new: messages.length };
}

async function runPool(tasks, concurrency) {
  const results = [];
  const executing = new Set();

  for (const task of tasks) {
    const p = task().then(r => {
      executing.delete(p);
      return r;
    });
    executing.add(p);
    results.push(p);

    if (executing.size >= concurrency) {
      await Promise.race(executing);
    }
  }

  return Promise.all(results);
}

export async function run(args) {
  const { flags, args: remaining } = parseGlobalFlags(args);
  const opts = parseArgs(remaining);

  if (args.includes('--help') || args.includes('-h') || !opts.guild) {
    console.log(`Usage: discord export --guild <id|name> [options]

Export Discord channels to local JSON files.

Options:
  --guild <id|name>     Server to export (required)
  --channel <name|id>   Export a single channel
  --threads             Include active threads
  --full                Full re-export (ignore incremental state)
  --concurrency <n>     Parallel channel exports (default: 4)
  --json                JSON output
  --help                Show help

Exports are incremental by default — only new messages are fetched.
Files saved to: ~/.discord-cli/exports/<server-name>/`);
    return;
  }

  const guildId = await client.resolveGuildId(opts.guild);
  const guilds = await client.getGuilds();
  const guild = guilds.find(g => g.id === guildId);
  const serverName = guild?.name || opts.guild;

  process.stderr.write(`Exporting: ${serverName}\n`);

  let channels = await client.getChannels(guildId);
  // Filter to text-based channels (text, announcement, forum)
  channels = channels.filter(c => [0, 5, 15].includes(c.type));

  if (opts.channel) {
    const channelId = await client.resolveChannelId(guildId, opts.channel);
    channels = channels.filter(c => c.id === channelId);
  }

  if (opts.threads) {
    try { const threads = await client.getActiveThreads(guildId); channels.push(...threads); } catch { /* bot-only on some servers */ }
  }

  process.stderr.write(`Channels to export: ${channels.length}\n\n`);

  const tasks = channels.map(ch => () => exportChannel(guildId, ch, serverName, opts));
  const results = await runPool(tasks, opts.concurrency);

  const summary = {
    server: serverName,
    channels: results.length,
    totalMessages: results.reduce((s, r) => s + (r.messages || 0), 0),
    newMessages: results.reduce((s, r) => s + (r.new || 0), 0),
    exportDir: store.getExportDir(serverName),
  };

  process.stderr.write(`\nDone. ${summary.totalMessages} total messages (${summary.newMessages} new)\n`);
  process.stderr.write(`Saved to: ${summary.exportDir}\n`);

  if (flags.json) {
    output(summary, flags);
  }
}
