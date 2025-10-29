import type { StructuredLogEvent } from './structured-log-event.js';

import { buildRichLogLine } from './rich-format.js';

interface FormatOptions {
  color?: boolean;
  verbose?: boolean;
}

const ANSI_RESET = '\u001B[0m';
const ANSI_RED = '\u001B[31m';
const ANSI_YELLOW = '\u001B[33m';
const ANSI_GREEN = '\u001B[32m';
const ANSI_BLUE = '\u001B[34m';

export function formatConsole(event: StructuredLogEvent, options: FormatOptions = {}): string {
  const rich = buildRichLogLine(event);
  const needsFullLineColor = rich.highlight === 'error' || rich.highlight === 'warning';

  let output = '';
  if (needsFullLineColor && options.color === true) {
    output = rich.highlight === 'error' ? ANSI_RED : ANSI_YELLOW;
  }

  output += rich.prefix;

  const shouldColorContext = options.color === true
    && !needsFullLineColor
    && rich.context !== undefined
    && rich.context.length > 0
    && rich.contextHighlight !== undefined;

  if (rich.context !== undefined && rich.context.length > 0) {
    if (shouldColorContext) {
      const color = rich.contextHighlight === 'llm' ? ANSI_BLUE : ANSI_GREEN;
      output += `${color}${rich.context}${ANSI_RESET}`;
    } else {
      output += rich.context;
    }
  }

  const needsSeparator = rich.context !== undefined
    && rich.context.length > 0
    && rich.message.length > 0;
  if (needsSeparator) {
    output += ' ';
  }

  output += rich.message;

  if (event.severity === 'ERR' && typeof event.stack === 'string' && event.stack.length > 0) {
    const stackLines = event.stack.split('\n').map((line) => `    ${line}`).join('\n');
    output += `\n${stackLines}`;
  }

  if (needsFullLineColor && options.color === true) {
    output += ANSI_RESET;
  }

  return output;
}
