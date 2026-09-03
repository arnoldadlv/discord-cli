import * as client from '../client.js';
import { parseGlobalFlags, table, output } from '../formatter.js';

export async function run(args) {
  const { flags } = parseGlobalFlags(args);

  if (args.includes('--help') || args.includes('-h')) {
    console.log(`Usage: discord dms [options]

List your DM conversations.

Options:
  --json    JSON output
  --help    Show help`);
    return;
  }

  const dms = await client.getDMs();

  const rows = dms
    .filter(dm => dm.type === 1 || dm.type === 3)
    .map(dm => ({
      id: dm.id,
      type: dm.type === 1 ? 'DM' : 'Group',
      name: dm.type === 3
        ? (dm.name || dm.recipients?.map(r => r.username).join(', '))
        : dm.recipients?.[0]?.username || 'Unknown',
      last_message_id: dm.last_message_id,
    }));

  if (flags.json) {
    output(rows, flags);
    return;
  }

  console.log(table(rows, [
    { key: 'id', label: 'ID', width: 22 },
    { key: 'type', label: 'Type', width: 6 },
    { key: 'name', label: 'Name', width: 30 },
  ]));
}
