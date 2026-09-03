import * as client from '../client.js';
import * as store from '../store.js';
import { readFile } from 'fs/promises';
import { basename, join } from 'path';
import { parseGlobalFlags, output } from '../formatter.js';

function parseArgs(args) {
  const opts = { limit: 25 };
  let i = 0;
  while (i < args.length) {
    const a = args[i];
    if (a === '--guild') opts.guild = args[++i];
    else if (a === '--dm') opts.dm = args[++i];
    else if (a === '--channel') opts.channel = args[++i];
    else if (a === '--query' || a === '-q') opts.query = args[++i];
    else if (a === '--author') opts.author = args[++i];
    else if (a === '--after') opts.after = args[++i];
    else if (a === '--before') opts.before = args[++i];
    else if (a === '--has') opts.has = args[++i];
    else if (a === '--limit') opts.limit = parseInt(args[++i], 10);
    else if (a === '--local') opts.local = true;
    else if (a === '--all') opts.all = true;
    else if (a === '--offset') opts.offset = parseInt(args[++i], 10);
    else if (!a.startsWith('-') && !opts.query) opts.query = a;
    i++;
  }
  return opts;
}

// --- Live search ---

async function liveSearchGuild(opts, flags) {
  const guildId = await client.resolveGuildId(opts.guild);
  const searchOpts = { sort_by: 'timestamp', sort_order: 'desc' };

  if (opts.query) searchOpts.content = opts.query;
  if (opts.has) searchOpts.has = opts.has;
  if (opts.limit) searchOpts.limit = Math.min(opts.limit, 25);
  if (opts.offset) searchOpts.offset = opts.offset;

  if (opts.channel) {
    searchOpts.channel_id = await client.resolveChannelId(guildId, opts.channel);
  }

  const data = await client.searchGuild(guildId, searchOpts);
  const messages = (data.messages || []).flat();

  const results = messages.map(m => ({
    id: m.id,
    channel_id: m.channel_id,
    author: m.author?.username || 'Unknown',
    timestamp: m.timestamp,
    content: m.content || '',
    attachments: m.attachments?.length || 0,
    reactions: m.reactions?.length || 0,
  }));

  if (flags.json) {
    output({ total: data.total_results, shown: results.length, results }, flags);
    return;
  }

  if (results.length === 0) {
    console.log('No results found.');
    return;
  }

  console.log(`Found ${data.total_results} results (showing ${results.length})\n`);
  for (const r of results) {
    const ts = new Date(r.timestamp).toLocaleString();
    console.log(`[${ts}] ${r.author} (${r.channel_id})`);
    console.log(r.content);
    if (r.attachments) console.log(`  [${r.attachments} attachment(s)]`);
    console.log('-'.repeat(60));
  }
}

async function liveSearchDM(opts, flags) {
  // No DM search endpoint — paginate and filter locally
  const dms = await client.getDMs();
  const lower = opts.dm.toLowerCase();
  const dm = dms.find(d =>
    d.recipients?.some(r => r.username?.toLowerCase().includes(lower))
  );
  if (!dm) throw new Error(`DM not found for: "${opts.dm}"`);

  const messages = await client.getAllMessages(dm.id, {}, (page, total) => {
    process.stderr.write(`\rFetching messages... ${total}`);
  });
  process.stderr.write('\n');

  const filtered = messages.filter(m => {
    if (opts.query) {
      const content = (m.content || '').toLowerCase();
      const terms = opts.query.toLowerCase().split(/\s+/);
      if (!terms.some(t => content.includes(t))) return false;
    }
    if (opts.after && new Date(m.timestamp) < new Date(opts.after)) return false;
    if (opts.before && new Date(m.timestamp) > new Date(opts.before)) return false;
    return true;
  });

  const results = filtered.slice(0, opts.limit).map(m => ({
    id: m.id,
    author: m.author?.username || 'Unknown',
    timestamp: m.timestamp,
    content: m.content || '',
    attachments: m.attachments?.length || 0,
  }));

  if (flags.json) {
    output({ total: filtered.length, shown: results.length, results }, flags);
    return;
  }

  if (results.length === 0) {
    console.log('No results found.');
    return;
  }

  console.log(`Found ${filtered.length} results (showing ${results.length})\n`);
  for (const r of results) {
    const ts = new Date(r.timestamp).toLocaleString();
    console.log(`[${ts}] ${r.author}`);
    console.log(r.content);
    console.log('-'.repeat(60));
  }
}

// --- Local search ---

async function localSearch(opts, flags) {
  let files = [];
  if (opts.guild) {
    files = await store.getExportFiles(opts.guild);
  } else if (opts.all) {
    const dirs = await store.getExportDirs();
    for (const d of dirs) {
      const serverFiles = await store.getExportFiles(d.name);
      files.push(...serverFiles);
    }
  }

  if (files.length === 0) {
    console.error('No export files found. Run `discord export` first or use live search (omit --local).');
    process.exit(1);
  }

  let allResults = [];
  for (const filePath of files) {
    try {
      const data = JSON.parse(await readFile(filePath, 'utf-8'));
      const messages = data.messages || [];
      const matches = messages.filter(msg => {
        if (opts.query) {
          const content = (msg.content || '').toLowerCase();
          const terms = opts.query.toLowerCase().split(/\s+/);
          if (!terms.some(t => content.includes(t))) return false;
        }
        if (opts.author) {
          const name = (msg.author?.nickname || msg.author?.name || msg.author?.username || '').toLowerCase();
          if (!name.includes(opts.author.toLowerCase())) return false;
        }
        if (opts.after && new Date(msg.timestamp) < new Date(opts.after)) return false;
        if (opts.before && new Date(msg.timestamp) > new Date(opts.before)) return false;
        return true;
      });

      allResults.push(...matches.map(msg => ({
        file: basename(filePath),
        server: basename(join(filePath, '..')),
        channel: data.channel?.name || 'Unknown',
        author: msg.author?.nickname || msg.author?.name || msg.author?.username || 'Unknown',
        timestamp: msg.timestamp,
        content: msg.content || '',
      })));
    } catch { /* skip bad files */ }
  }

  allResults.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
  const limited = allResults.slice(0, opts.limit);

  if (flags.json) {
    output({ total: allResults.length, shown: limited.length, results: limited }, flags);
    return;
  }

  if (allResults.length === 0) {
    console.log('No matches found.');
    return;
  }

  console.log(`Found ${allResults.length} matches across ${files.length} file(s)\n`);
  for (const r of limited) {
    const ts = new Date(r.timestamp).toLocaleString();
    console.log(`[${ts}] ${r.author} — #${r.channel} (${r.server})`);
    console.log(r.content);
    console.log('-'.repeat(60));
  }

  if (allResults.length > opts.limit) {
    console.log(`\n... and ${allResults.length - opts.limit} more (use --limit to see more)`);
  }
}

// --- Entry point ---

export async function run(args) {
  const { flags, args: remaining } = parseGlobalFlags(args);
  const opts = parseArgs(remaining);

  if (args.includes('--help') || args.includes('-h')) {
    console.log(`Usage: discord search [options]

Search Discord messages live or in local exports.

Modes:
  --guild <name|id>    Live search a server (default)
  --dm <username>      Live search a DM conversation
  --local              Search local exports instead of live
  --all                Search all local exports (with --local)

Filters:
  --query, -q <text>   Search text
  --author <name>      Filter by author (local only)
  --channel <name|id>  Scope to a channel
  --after <date>       Messages after date (YYYY-MM-DD)
  --before <date>      Messages before date (YYYY-MM-DD)
  --has <type>         Has: attachment, link, embed, image, video
  --limit <n>          Max results (default: 25)
  --offset <n>         Offset for pagination (live only)

Output:
  --json               JSON output
  --help               Show help

Examples:
  discord search --guild cooey-coe --query "access control"
  discord search --dm "Kyle" --query "meeting"
  discord search --local --guild cooey-coe --query "MFA"
  discord search --local --all --query "policy"`);
    return;
  }

  if (opts.local) {
    await localSearch(opts, flags);
  } else if (opts.dm) {
    await liveSearchDM(opts, flags);
  } else if (opts.guild) {
    await liveSearchGuild(opts, flags);
  } else {
    console.error('Specify --guild <name>, --dm <user>, or --local with --guild/--all');
    process.exit(1);
  }
}
