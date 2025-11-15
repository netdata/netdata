import type { StructuredLogEvent } from './structured-log-event.js';

const SEVERITY_LABELS: Record<StructuredLogEvent['severity'], string> = {
  ERR: 'ERR',
  WRN: 'WRN',
  FIN: 'FIN',
  VRB: 'VRB',
  THK: 'THK',
  TRC: 'TRC',
};

const ANSI_RESET = '\u001B[0m';
const ANSI_RED = '\u001B[31m';
const ANSI_YELLOW = '\u001B[33m';
const ANSI_GREEN = '\u001B[32m';
const ANSI_BLUE = '\u001B[34m';

type HighlightKind = 'error' | 'warning' | 'llm' | 'tool' | undefined;

interface RichLogLine {
  prefix: string;
  context?: string;
  message: string;
  highlight: HighlightKind;
  contextHighlight: 'llm' | 'tool' | undefined;
}

function extractToolParams(event: StructuredLogEvent): string {
  const preview = event.labels.request_preview;
  if (typeof preview === 'string' && preview.length > 0) {
    const parenIndex = preview.indexOf('(');
    if (parenIndex > 0) {
      return preview.slice(parenIndex);
    }
    return `(${preview})`;
  }

  if (event.message.includes('(') && event.message.includes(')')) {
    const regex = /\(([^)]+)\)/;
    const match = regex.exec(event.message);
    if (match !== null) {
      return `(${match[1]})`;
    }
  }
  return '';
}

function parseNumericLabel(event: StructuredLogEvent, key: string): number | undefined {
  if (!Object.prototype.hasOwnProperty.call(event.labels, key)) return undefined;
  const raw = event.labels[key];
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) return undefined;
  return parsed;
}

function extractLegacyMetrics(message: string): string[] {
  const metrics: string[] = [];

  const latencyRegex = /(\d+(?:\.\d+)?)\s*ms/;
  const latencyMatch = latencyRegex.exec(message);
  const latency: string | undefined = latencyMatch !== null ? latencyMatch[1] : undefined;
  if (latency !== undefined) {
    metrics.push(`${latency}ms`);
  }

  const bytesRegex = /(\d+)\s*bytes/;
  const bytesMatch = bytesRegex.exec(message);
  const bytes: string | undefined = bytesMatch !== null ? bytesMatch[1] : undefined;
  if (bytes !== undefined) {
    metrics.push(`${bytes} bytes`);
  }

  const tokensRegex = /(\d+)\s*tokens/;
  const tokensMatch = tokensRegex.exec(message);
  const tokens: string | undefined = tokensMatch !== null ? tokensMatch[1] : undefined;
  if (tokens !== undefined) {
    metrics.push(`${tokens} tokens`);
  }

  return metrics;
}

function getKindCode(event: StructuredLogEvent): string {
  if (event.type === 'llm') return 'LLM';
  if (event.toolKind === 'mcp') return 'MCP';
  if (event.toolKind === 'rest') return 'RST';
  if (event.toolKind === 'agent') return 'AGN';
  if (event.remoteIdentifier?.startsWith('agent:') === true) return 'INT';
  return 'INT';
}

function buildContext(event: StructuredLogEvent): { text?: string; highlight: 'llm' | 'tool' | undefined } {
  if (event.type === 'llm') {
    const buildProviderModelLabel = (): string | undefined => {
      const provider = event.provider ?? '';
      const model = event.model ?? '';
      if (provider.length > 0 && model.length > 0) {
        return `${provider}/${model}`;
      }
      return undefined;
    };
    if (event.direction === 'response') {
      const message = typeof event.message === 'string' ? event.message : '';
      const isResponseMetricLog = message === 'LLM response received';
      const isFailureMetricLog = message.startsWith('LLM response failed');
      if (isResponseMetricLog || isFailureMetricLog) {
        const labelPrefix = isFailureMetricLog ? 'LLM response failed' : 'LLM response received';
      const getNumber = (key: string): number | undefined => {
        if (!Object.prototype.hasOwnProperty.call(event.labels, key)) return undefined;
        const raw = event.labels[key];
        const parsed = Number(raw);
        return Number.isFinite(parsed) ? parsed : undefined;
      };
      const getString = (key: string): string | undefined => {
        if (!Object.prototype.hasOwnProperty.call(event.labels, key)) return undefined;
        const raw = event.labels[key];
        return typeof raw === 'string' && raw.length > 0 ? raw : undefined;
      };

      const cost = getNumber('cost_usd');
      const latencyMs = getNumber('latency_ms');
      const bytes = getNumber('response_bytes');
      const inputTokens = getNumber('input_tokens');
      const outputTokens = getNumber('output_tokens');
      const cacheRead = getNumber('cache_read_tokens');
      const cacheWrite = getNumber('cache_write_tokens');
      const ctxTokens = getNumber('ctx_tokens');
      const stopReason = getString('stop_reason');
      const reasoning = getString('reasoning');

      const parts: string[] = [];
      if (typeof cost === 'number') parts.push(`$${cost.toFixed(6)}`);
      if (typeof latencyMs === 'number') parts.push(`${String(latencyMs)}ms`);
      if (typeof bytes === 'number') parts.push(`${String(bytes)} bytes`);

      const tokenSegments: string[] = [];
      if (typeof inputTokens === 'number') tokenSegments.push(`in ${String(inputTokens)}`);
      if (typeof outputTokens === 'number') tokenSegments.push(`out ${String(outputTokens)}`);
      if (typeof cacheRead === 'number') tokenSegments.push(`cR ${String(cacheRead)}`);
      if (typeof cacheWrite === 'number') tokenSegments.push(`cW ${String(cacheWrite)}`);
      if (tokenSegments.length > 0) {
        parts.push(`tokens: ${tokenSegments.join(', ')}`);
      }
      if (typeof ctxTokens === 'number') {
        parts.push(`ctx ${String(ctxTokens)}`);
      }
      if (typeof stopReason === 'string') parts.push(`stop=${stopReason}`);
      if (typeof reasoning === 'string') parts.push(`reasoning=${reasoning}`);

        const reconstructed = parts.length > 0
          ? `${labelPrefix} [${parts.join(', ')}]`
          : labelPrefix;
        return { text: reconstructed, highlight: 'llm' };
      }
      const providerLabel = buildProviderModelLabel();
      if (providerLabel !== undefined) {
        return { text: providerLabel, highlight: 'llm' };
      }
      return { highlight: 'llm' };
    }

    const providerLabel = buildProviderModelLabel();
    if (providerLabel !== undefined) {
      return { text: providerLabel, highlight: 'llm' };
    }
    return { highlight: 'llm' };
  }

  let namespaceLabel: string | undefined;
  let simpleTool: string | undefined;
  if (event.remoteIdentifier?.includes(':') === true) {
    const parts = event.remoteIdentifier.split(':');
    if (parts.length >= 3) {
      [ , namespaceLabel ] = parts;
      simpleTool = parts.slice(2).join(':');
    } else if (parts.length === 2) {
      [namespaceLabel] = parts;
      simpleTool = parts[1];
    }
  }
  if (simpleTool === undefined) {
    const labelTool = event.labels.tool;
    if (typeof labelTool === 'string' && labelTool.length > 0) {
      simpleTool = labelTool;
    }
  }
  if (namespaceLabel === undefined) {
    const nsLabel = event.labels.tool_namespace;
    if (typeof nsLabel === 'string' && nsLabel.length > 0) {
      namespaceLabel = nsLabel;
    }
  }
  let displayLabel: string | undefined;
  if (namespaceLabel !== undefined && simpleTool !== undefined) {
    displayLabel = `${namespaceLabel}:${simpleTool}`;
  } else if (simpleTool !== undefined) {
    displayLabel = simpleTool;
  }

  if (displayLabel === undefined) {
    return { highlight: 'tool' };
  }

  if (event.direction === 'request') {
    const params = extractToolParams(event);
    return { text: `${displayLabel}${params}`, highlight: 'tool' };
  }

  let context = displayLabel;
  const metrics: string[] = [];
  const latency = parseNumericLabel(event, 'latency_ms');
  if (typeof latency === 'number') metrics.push(`${String(latency)}ms`);
  const bytes = parseNumericLabel(event, 'result_bytes')
    ?? parseNumericLabel(event, 'output_bytes')
    ?? parseNumericLabel(event, 'characters_out');
  if (typeof bytes === 'number') metrics.push(`${String(bytes)} bytes`);
  const tokens = parseNumericLabel(event, 'tokens_estimated') ?? parseNumericLabel(event, 'tokens');
  if (typeof tokens === 'number') metrics.push(`${String(tokens)} tokens`);
  const flags: string[] = [];
  if (event.labels.truncated === 'true') flags.push('truncated');
  if (event.labels.dropped === 'true') flags.push('overflown');
  if (metrics.length > 0 || flags.length > 0) {
    const segments = [...metrics];
    if (flags.length > 0) segments.push(flags.join('|'));
    context += ` [${segments.join(', ')}]`;
  } else {
    const legacyMetrics = extractLegacyMetrics(event.message);
    if (legacyMetrics.length > 0) {
      context += ` [${legacyMetrics.join(', ')}]`;
    }
  }
  return { text: context, highlight: 'tool' };
}

function buildRichLogLine(event: StructuredLogEvent): RichLogLine {
  const severityLabel = SEVERITY_LABELS[event.severity];
  const turnStr = event.turnPath ?? `${String(event.turn)}.${String(event.subturn)}`;
  const direction = event.direction === 'request' ? '→' : '←';
  const kind = getKindCode(event);
  const agent = event.agentPath ?? event.agentId ?? 'main';
  const prefix = `${severityLabel} ${turnStr} ${direction} ${kind} ${agent}: `;

  const { text: context, highlight } = buildContext(event);

  const lineHighlight: HighlightKind = event.severity === 'ERR'
    ? 'error'
    : event.severity === 'WRN'
      ? 'warning'
      : highlight;

  const contextText = (context !== undefined && context.length > 0) ? context : undefined;
  const body = event.message.trim();
  const contextHighlight = contextText !== undefined ? highlight : undefined;

  return {
    prefix,
    context: contextText,
    message: body,
    highlight: lineHighlight,
    contextHighlight,
  };
}

export interface FormatRichLogLineOptions {
  tty?: boolean;
}

export function formatRichLogLine(
  event: StructuredLogEvent,
  options: FormatRichLogLineOptions = {},
): string {
  const rich = buildRichLogLine(event);
  const useColor = options.tty === true;
  const needsFullLineColor = useColor && (rich.highlight === 'error' || rich.highlight === 'warning');

  let output = '';
  if (needsFullLineColor) {
    output = rich.highlight === 'error' ? ANSI_RED : ANSI_YELLOW;
  }

  output += rich.prefix;

  const shouldColorContext = useColor
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

  if (needsFullLineColor) {
    output += ANSI_RESET;
  }

  return output;
}
