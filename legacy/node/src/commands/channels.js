import * as client from '../client.js';
import { parseGlobalFlags, output } from '../formatter.js';

const TYPE_LABELS = {
  0: 'text', 2: 'voice', 4: 'category', 5: 'announcement',
  10: 'ann-thread', 11: 'thread', 12: 'priv-thread',
  13: 'stage', 15: 'forum', 16: 'media',
};

function parseArgs(args) {
  const opts = { threads: false };
  let i = 0;
  while (i < args.length) {
    if (args[i] === '--guild') opts.guild = args[++i];
    else if (args[i] === '--threads') opts.threads = true;
    i++;
  }
  return opts;
}

export async function run(args) {
  const { flags, args: remaining } = parseGlobalFlags(args);
  const opts = parseArgs(remaining);

  if (args.includes('--help') || args.includes('-h') || !opts.guild) {
    console.log(`Usage: discord channels --guild <id|name> [options]

List channels in a server.

Options:
  --guild <id|name>   Server to list (required)
  --threads           Include active threads
  --json              JSON output
  --help              Show help`);
    return;
  }

  const guildId = await client.resolveGuildId(opts.guild);
  const channels = await client.getChannels(guildId);

  let threads = [];
  if (opts.threads) {
    try { threads = await client.getActiveThreads(guildId); } catch { /* bot-only on some servers */ }
  }

  const all = [...channels, ...threads];

  if (flags.json) {
    output(all.map(c => ({
      id: c.id,
      name: c.name,
      type: TYPE_LABELS[c.type] || c.type,
      parent_id: c.parent_id || null,
      position: c.position,
    })), flags);
    return;
  }

  // Group by category for human output
  const categories = channels.filter(c => c.type === 4).sort((a, b) => a.position - b.position);
  const uncategorized = all.filter(c => !c.parent_id && c.type !== 4);

  if (uncategorized.length) {
    console.log('(no category)');
    for (const ch of uncategorized) {
      console.log(`  #${ch.name}  ${TYPE_LABELS[ch.type] || ''}  ${ch.id}`);
    }
  }

  for (const cat of categories) {
    console.log(`\n${cat.name.toUpperCase()}`);
    const children = all.filter(c => c.parent_id === cat.id).sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    for (const ch of children) {
      const prefix = [11, 12, 10].includes(ch.type) ? '  └ ' : '  #';
      console.log(`${prefix}${ch.name}  ${TYPE_LABELS[ch.type] || ''}  ${ch.id}`);
    }
  }
}
