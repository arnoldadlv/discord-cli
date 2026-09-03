#!/usr/bin/env node

import { argv, exit } from 'process';

const COMMANDS = {
  guilds: () => import('../src/commands/guilds.js'),
  channels: () => import('../src/commands/channels.js'),
  dms: () => import('../src/commands/dms.js'),
  messages: () => import('../src/commands/messages.js'),
  search: () => import('../src/commands/search.js'),
  export: () => import('../src/commands/export.js'),
  info: () => import('../src/commands/info.js'),
};

const args = argv.slice(2);
const command = args[0];

if (!command || command === '--help' || command === '-h') {
  console.log(`Usage: discord <command> [options]

Commands:
  guilds              List servers you're in
  channels            List channels in a server
  dms                 List DM conversations
  messages            Show recent messages in a channel or DM
  search              Search messages (live or local)
  export              Export channels to local JSON
  info                Server summary and export status

Options:
  --help, -h          Show help
  --json              Machine-readable JSON output

Run 'discord <command> --help' for command-specific help.`);
  exit(0);
}

if (!COMMANDS[command]) {
  console.error(`Unknown command: ${command}`);
  console.error(`Run 'discord --help' for available commands.`);
  exit(1);
}

const mod = await COMMANDS[command]();
await mod.run(args.slice(1));
