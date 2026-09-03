import * as client from '../client.js';
import * as store from '../store.js';
import { parseGlobalFlags, output } from '../formatter.js';

function parseArgs(args) {
  const opts = {};
  let i = 0;
  while (i < args.length) {
    if (args[i] === '--guild') opts.guild = args[++i];
    i++;
  }
  return opts;
}

export async function run(args) {
  const { flags, args: remaining } = parseGlobalFlags(args);
  const opts = parseArgs(remaining);

  if (args.includes('--help') || args.includes('-h') || !opts.guild) {
    console.log(`Usage: discord info --guild <id|name>

Show server summary and export status.

Options:
  --guild <id|name>   Server (required)
  --json              JSON output
  --help              Show help`);
    return;
  }

  const guildId = await client.resolveGuildId(opts.guild);
  const [guild, channels] = await Promise.all([
    client.getGuild(guildId),
    client.getChannels(guildId),
  ]);
  let threads = [];
  try { threads = await client.getActiveThreads(guildId); } catch { /* bot-only on some servers */ }

  const meta = await store.getMeta(guild.name || opts.guild);
  const exportFiles = await store.getExportFiles(store.normalizeName(guild.name || opts.guild));

  const textChannels = channels.filter(c => [0, 5, 15].includes(c.type));
  const voiceChannels = channels.filter(c => [2, 13].includes(c.type));
  const categories = channels.filter(c => c.type === 4);

  const info = {
    id: guild.id,
    name: guild.name,
    members: guild.approximate_member_count,
    online: guild.approximate_presence_count,
    channels: {
      text: textChannels.length,
      voice: voiceChannels.length,
      categories: categories.length,
      threads: threads.length,
    },
    export: {
      lastExport: meta.lastExport,
      channelsExported: Object.keys(meta.channels).length,
      filesOnDisk: exportFiles.length,
    },
  };

  if (flags.json) {
    output(info, flags);
    return;
  }

  console.log(`${info.name} (${info.id})`);
  console.log(`Members: ${info.members} (${info.online} online)`);
  console.log(`Channels: ${info.channels.text} text, ${info.channels.voice} voice, ${info.channels.categories} categories, ${info.channels.threads} active threads`);
  console.log(`\nExport status:`);
  console.log(`  Last export: ${info.export.lastExport || 'never'}`);
  console.log(`  Channels exported: ${info.export.channelsExported}`);
  console.log(`  Files on disk: ${info.export.filesOnDisk}`);
}
