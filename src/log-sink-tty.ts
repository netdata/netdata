import type { AIAgentCallbacks, LogEntry } from './types.js';

import { createStructuredLogger } from './logging/structured-logger.js';
import { getTelemetryLabels } from './telemetry/index.js';

export function makeTTYLogCallbacks(
  opts: { color?: boolean; verbose?: boolean; traceLlm?: boolean; traceMcp?: boolean; traceSdk?: boolean },
  write?: (s: string) => void
): Pick<AIAgentCallbacks, 'onLog'> {
  const logfmtWriter = typeof write === 'function'
    ? write
    : (s: string) => {
        try {
          process.stderr.write(s);
        } catch (e) {
          try { process.stderr.write(`[warn] tty write failed: ${e instanceof Error ? e.message : String(e)}\n`); } catch {}
        }
      };

  const baseLabels = { ...getTelemetryLabels() };
  const logger = createStructuredLogger({
    color: opts.color ?? process.stderr.isTTY,
    logfmtWriter,
    labels: baseLabels,
  });

  return {
    onLog: (entry: LogEntry) => {
      if (entry.severity === 'THK') return;
      if (entry.severity === 'VRB' && opts.verbose !== true) return;
      if (entry.severity === 'TRC') {
        const traceLlm = opts.traceLlm === true || opts.traceSdk === true;
        const traceMcp = opts.traceMcp === true;
        const isLlm = entry.type === 'llm';
        const isTool = entry.type === 'tool';
        if ((isLlm && !traceLlm) || (isTool && !traceMcp)) return;
      }
      logger.emit(entry);
    },
  };
}
