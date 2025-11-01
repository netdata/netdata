import type { StructuredLogEvent } from './structured-log-event.js';

import { formatRichLogLine } from './rich-format.js';

interface FormatOptions {
  color?: boolean;
  verbose?: boolean;
}

export function formatConsole(event: StructuredLogEvent, options: FormatOptions = {}): string {
  const outputLine = formatRichLogLine(event, { tty: options.color === true });
  let output = outputLine;

  if (event.severity === 'ERR' && typeof event.stack === 'string' && event.stack.length > 0) {
    const stackLines = event.stack.split('\n').map((line) => `    ${line}`).join('\n');
    output += `\n${stackLines}`;
  }

  return output;
}
