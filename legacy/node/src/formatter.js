export function parseGlobalFlags(args) {
  const flags = { json: false, compact: false };
  const remaining = [];
  for (const arg of args) {
    if (arg === '--json') flags.json = true;
    else if (arg === '--compact') flags.compact = true;
    else remaining.push(arg);
  }
  return { flags, args: remaining };
}

export function table(rows, columns) {
  // columns: [{ key, label, width?, align? }]
  const widths = columns.map(col => {
    const max = Math.max(col.label.length, ...rows.map(r => String(r[col.key] ?? '').length));
    return col.width || Math.min(max, 40);
  });

  const header = columns.map((col, i) => col.label.padEnd(widths[i])).join('  ');
  const sep = widths.map(w => '-'.repeat(w)).join('  ');
  const body = rows.map(row =>
    columns.map((col, i) => {
      const val = String(row[col.key] ?? '');
      return col.align === 'right' ? val.padStart(widths[i]) : val.padEnd(widths[i]);
    }).join('  ')
  );

  return [header, sep, ...body].join('\n');
}

export function output(data, flags) {
  if (flags.json) {
    console.log(JSON.stringify(data, null, 2));
  } else if (typeof data === 'string') {
    console.log(data);
  } else {
    console.log(data);
  }
}
