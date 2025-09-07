import type { AIAgentCallbacks, LogEntry } from './types.js';

import { formatLog } from './log-formatter.js';

export function makeTTYLogCallbacks(opts: { color?: boolean; verbose?: boolean; traceLlm?: boolean; traceMcp?: boolean }, write?: (s: string) => void): Pick<AIAgentCallbacks, 'onLog'> {
  const out = typeof write === 'function' ? write : (s: string) => { try { process.stderr.write(s); } catch { /* ignore */ } };
  return {
    onLog: (entry: LogEntry) => {
      const line = formatLog(entry, opts);
      if (line.length > 0 && entry.severity !== 'THK') out(`${line}\n`);
    }
  };
}
