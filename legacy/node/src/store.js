import { readFile, writeFile, readdir, stat, mkdir } from 'fs/promises';
import { join, basename } from 'path';
import { homedir } from 'os';

const EXPORT_DIR = join(homedir(), '.discord-cli', 'exports');
const LEGACY_DIR = join(homedir(), 'DiscordChatExporter.Cli.osx-arm64', 'exports');

export function normalizeName(name) {
  return name.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');
}

export function getExportDir(serverName) {
  return join(EXPORT_DIR, normalizeName(serverName));
}

export async function ensureDir(dir) {
  await mkdir(dir, { recursive: true });
}

// --- Metadata for incremental exports ---

async function getMetaPath(serverName) {
  return join(getExportDir(serverName), '.meta.json');
}

export async function getMeta(serverName) {
  try {
    const data = await readFile(await getMetaPath(serverName), 'utf-8');
    return JSON.parse(data);
  } catch {
    return { channels: {}, lastExport: null };
  }
}

export async function setChannelMeta(serverName, channelId, data) {
  const meta = await getMeta(serverName);
  meta.channels[channelId] = { ...meta.channels[channelId], ...data };
  meta.lastExport = new Date().toISOString();
  const metaPath = await getMetaPath(serverName);
  await ensureDir(getExportDir(serverName));
  await writeFile(metaPath, JSON.stringify(meta, null, 2));
}

// --- Reading exports (new + legacy) ---

export async function getExportDirs() {
  const dirs = [];
  for (const base of [EXPORT_DIR, LEGACY_DIR]) {
    try {
      const entries = await readdir(base);
      for (const entry of entries) {
        const full = join(base, entry);
        const s = await stat(full);
        if (s.isDirectory()) {
          dirs.push({ name: entry, path: full, legacy: base === LEGACY_DIR });
        }
      }
    } catch { /* dir doesn't exist */ }
  }
  return dirs;
}

export async function getExportFiles(serverName) {
  const files = [];
  for (const base of [EXPORT_DIR, LEGACY_DIR]) {
    const dir = join(base, serverName);
    try {
      const entries = await readdir(dir);
      for (const f of entries) {
        if (f.endsWith('.json') && f !== '.meta.json') {
          files.push(join(dir, f));
        }
      }
    } catch { /* dir doesn't exist */ }
  }
  // Also try normalized name in new dir
  const normalized = normalizeName(serverName);
  if (normalized !== serverName) {
    const dir = join(EXPORT_DIR, normalized);
    try {
      const entries = await readdir(dir);
      for (const f of entries) {
        if (f.endsWith('.json') && f !== '.meta.json') {
          const full = join(dir, f);
          if (!files.includes(full)) files.push(full);
        }
      }
    } catch { /* */ }
  }
  return files;
}

export async function readExport(filePath) {
  const data = await readFile(filePath, 'utf-8');
  return JSON.parse(data);
}

// --- Writing exports ---

export async function writeExport(serverName, channelName, data) {
  const dir = getExportDir(serverName);
  await ensureDir(dir);
  const filename = `${normalizeName(channelName)}.json`;
  await writeFile(join(dir, filename), JSON.stringify(data, null, 2));
  return join(dir, filename);
}
