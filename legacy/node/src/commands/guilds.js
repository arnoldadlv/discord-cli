import * as client from '../client.js';
import { parseGlobalFlags, table, output } from '../formatter.js';

export async function run(args) {
  const { flags } = parseGlobalFlags(args);

  if (args.includes('--help') || args.includes('-h')) {
    console.log(`Usage: discord guilds [options]

List all Discord servers you're a member of.

Options:
  --json    JSON output
  --help    Show help`);
    return;
  }

  const guilds = await client.getGuilds();

  if (flags.json) {
    output(guilds.map(g => ({
      id: g.id,
      name: g.name,
      members: g.approximate_member_count,
      owner: g.owner,
    })), flags);
    return;
  }

  console.log(table(
    guilds.map(g => ({
      id: g.id,
      name: g.name,
      members: g.approximate_member_count ?? '?',
    })),
    [
      { key: 'id', label: 'ID', width: 22 },
      { key: 'name', label: 'Name', width: 30 },
      { key: 'members', label: 'Members', width: 10, align: 'right' },
    ]
  ));
}
