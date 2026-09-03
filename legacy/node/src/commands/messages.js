import * as client from '../client.js';
import { parseGlobalFlags, output } from '../formatter.js';

function parseArgs(args) {
  const opts = { limit: 25 };
  let i = 0;
  while (i < args.length) {
    const a = args[i];
    if (a === '--guild') opts.guild = args[++i];
    else if (a === '--channel') opts.channel = args[++i];
    else if (a === '--dm') opts.dm = args[++i];
    else if (a === '--limit') opts.limit = parseInt(args[++i], 10);
    else if (a === '--before') opts.before = args[++i];
    else if (a === '--after') opts.after = args[++i];
    i++;
  }
  return opts;
}

function formatMessage(m) {
  const author = m.author?.global_name || m.author?.username || 'Unknown';
  const ts = new Date(m.timestamp).toLocaleString();
  const lines = [`[${ts}] ${author}`];

  if (m.content) lines.push(m.content);

  for (const embed of (m.embeds || [])) {
    if (embed.title) lines.push(`  [embed] ${embed.title}`);
    if (embed.description) lines.push(`  ${embed.description.replace(/<[^>]+>/g, '').slice(0, 300)}`);
    if (embed.url) lines.push(`  ${embed.url}`);
  }

  for (const att of (m.attachments || [])) {
    lines.push(`  [attachment] ${att.filename} (${att.url})`);
  }

  if (m.reactions?.length) {
    const rxns = m.reactions.map(r => `${r.emoji.name} ${r.count}`).join('  ');
    lines.push(`  reactions: ${rxns}`);
  }

  return lines.join('\n');
}

export async function run(args) {
  const { flags, args: remaining } = parseGlobalFlags(args);
  const opts = parseArgs(remaining);

  if (args.includes('--help') || args.includes('-h')) {
    console.log(`Usage: discord messages --guild <name> --channel <name> [options]
       discord messages --dm <username> [options]

Show recent messages in a channel or DM.

Options:
  --guild <name|id>     Server (required for channels)
  --channel <name|id>   Channel (required for guild messages)
  --dm <username>       DM conversation (partial match)
  --limit <n>           Number of messages (default: 25)
  --before <id>         Messages before this message ID
  --after <id>          Messages after this message ID
  --json                Full JSON output
  --help                Show help

Examples:
  discord messages --guild cooey-coe --channel general
  discord messages --guild cooey-coe --channel 📰news --limit 5
  discord messages --dm Kyle --limit 10`);
    return;
  }

  let channelId;

  if (opts.dm) {
    const dms = await client.getDMs();
    const lower = opts.dm.toLowerCase();
    const dm = dms.find(d =>
      d.recipients?.some(r =>
        r.username?.toLowerCase().includes(lower) ||
        r.global_name?.toLowerCase()?.includes(lower)
      )
    );
    if (!dm) throw new Error(`DM not found for: "${opts.dm}"`);
    channelId = dm.id;
  } else if (opts.guild && opts.channel) {
    const guildId = await client.resolveGuildId(opts.guild);
    channelId = await client.resolveChannelId(guildId, opts.channel);
  } else {
    console.error('Specify --guild <name> --channel <name>, or --dm <username>');
    process.exit(1);
  }

  const fetchOpts = { limit: Math.min(opts.limit, 100) };
  if (opts.before) fetchOpts.before = opts.before;
  if (opts.after) fetchOpts.after = opts.after;

  const messages = await client.getMessages(channelId, fetchOpts);

  if (flags.json) {
    output(messages, flags);
    return;
  }

  if (messages.length === 0) {
    console.log('No messages found.');
    return;
  }

  // Display oldest first for natural reading order
  const sorted = [...messages].reverse();
  console.log(sorted.map(formatMessage).join('\n' + '-'.repeat(60) + '\n'));
}
