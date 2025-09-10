import path from 'node:path';

import type { LogEntry } from './types.js';

interface LogFormatOptions {
  color?: boolean;
  verbose?: boolean;
  traceLlm?: boolean;
  traceMcp?: boolean;
}

const ansi = (enabled: boolean, code: string, s: string): string => (enabled ? `${code}${s}\x1b[0m` : s);

export function formatLog(entry: LogEntry, opts: LogFormatOptions = {}): string {
  let useColor = false;
  if (opts.color === true) useColor = true;
  else if (opts.color === false) useColor = false;
  else useColor = process.stderr.isTTY ? true : false;

  const dir = entry.direction === 'request' ? '→' : '←';
  const turn = `${String(entry.turn)}.${String(entry.subturn)}`;
  const agentPrefix = (() => {
    const a = entry.agentId;
    if (typeof a === 'string' && a.length > 0) {
      try { return `${path.basename(a)} `; } catch { return `${a} `; }
    }
    return '';
  })();
  const remote = (() => {
    const v = entry.remoteIdentifier;
    if (typeof v === 'string' && v.length > 0) return v;
    return 'unknown';
  })();

  // Filter
  if (entry.severity === 'VRB' && opts.verbose !== true) return '';
  if (entry.severity === 'TRC') {
    const tl = entry.type === 'llm' && opts.traceLlm === true;
    const tm = entry.type === 'tool' && opts.traceMcp === true;
    if (!tl && !tm) return '';
  }

  const base = (() => {
    if (remote === 'agent') return `agent ${entry.message}`;
    return `${entry.type} ${remote}: ${entry.message}`;
  })();

  const raw = `${agentPrefix}[${entry.severity}] ${dir} [${turn}] ${base}`;
  // Color by severity (support bold hint on VRB)
  switch (entry.severity) {
    case 'ERR': return ansi(useColor, '\x1b[31m', raw); // red
    case 'WRN': return ansi(useColor, '\x1b[33m', raw); // yellow
    case 'FIN': return ansi(useColor, '\x1b[36m', raw); // cyan
    case 'TRC': return ansi(useColor, '\x1b[90m', raw); // gray
    case 'VRB': {
      const bold = (entry as { bold?: boolean }).bold === true;
      const code = bold ? '\x1b[1;90m' : '\x1b[90m';
      return ansi(useColor, code, raw); // gray (bold when requested)
    }
    case 'THK': return ansi(useColor, '\x1b[2;37m', raw); // dim white
    default: return raw;
  }
}
