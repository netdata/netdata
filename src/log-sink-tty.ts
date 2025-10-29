import type { AIAgentCallbacks, LogEntry } from './types.js';

import { createStructuredLogger, type LogFormat } from './logging/structured-logger.js';
import { getTelemetryLabels } from './telemetry/index.js';

export function makeTTYLogCallbacks(
  opts: {
    color?: boolean;
    verbose?: boolean;
    traceLlm?: boolean;
    traceMcp?: boolean;
    traceSdk?: boolean;
    serverMode?: boolean;
    explicitFormat?: string;
  },
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

  // Determine format based on context
  let formats: LogFormat[] | undefined;

  if (opts.explicitFormat !== undefined && opts.explicitFormat.length > 0) {
    // User explicitly requested a format
    formats = [opts.explicitFormat as LogFormat];
  } else if (process.stderr.isTTY && opts.serverMode !== true) {
    // Interactive console mode - use simplified format
    formats = ['console'];
  } else {
    // Server mode or non-TTY - use default selection logic
    formats = undefined;
  }

  const logger = createStructuredLogger({
    formats,
    color: opts.color ?? process.stderr.isTTY,
    verbose: opts.verbose === true,
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
