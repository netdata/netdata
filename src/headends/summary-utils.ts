import type { AgentMetadata } from '../agent-registry.js';
import type { ProgressMetrics } from '../types.js';

export const escapeMarkdown = (value: string): string => value
  .replace(/\r/gu, '')
  .replace(/([*_`])/gu, '\\$1')
  .trim();

export const italicize = (value: string): string => {
  const escaped = escapeMarkdown(value);
  return escaped.length > 0 ? `_${escaped}_` : escaped;
};

export const formatDuration = (ms?: number): string | undefined => {
  if (ms === undefined || !Number.isFinite(ms) || ms <= 0) return undefined;
  const seconds = Math.max(1, Math.round(ms / 1000));
  if (seconds < 60) return `${String(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return remaining > 0
    ? `${String(minutes)}m${String(remaining)}s`
    : `${String(minutes)}m`;
};

export const formatTokenSegments = (metrics: ProgressMetrics): string | undefined => {
  const segments: string[] = [];
  const push = (label: string, value: number | undefined): void => {
    if (typeof value === 'number' && Number.isFinite(value)) segments.push(`${label}${String(value)}`);
  };
  push('→', metrics.tokensIn);
  push('←', metrics.tokensOut);
  push('c→', metrics.tokensCacheRead);
  push('c←', metrics.tokensCacheWrite);
  if (segments.length === 0) return undefined;
  return `tokens ${segments.join(' ')}`;
};

export const formatMetricsLine = (metrics?: ProgressMetrics): string => {
  if (metrics === undefined) return '';
  const parts: string[] = [];
  const duration = formatDuration(metrics.durationMs);
  if (typeof duration === 'string' && duration.length > 0) parts.push(`duration **${escapeMarkdown(duration)}**`);
  const tokens = formatTokenSegments(metrics);
  if (tokens !== undefined) parts.push(tokens);
  if (typeof metrics.toolsRun === 'number' && Number.isFinite(metrics.toolsRun)) parts.push(`tools ${String(metrics.toolsRun)}`);
  if (typeof metrics.agentsRun === 'number' && Number.isFinite(metrics.agentsRun)) parts.push(`agents ${String(metrics.agentsRun)}`);
  if (typeof metrics.costUsd === 'number' && Number.isFinite(metrics.costUsd)) parts.push(`cost $**${metrics.costUsd.toFixed(4)}**`);
  return parts.join(', ');
};

export interface SummaryLineOptions {
  agentLabel: string;
  metrics?: ProgressMetrics;
  statusNote?: string;
  usageSnapshot?: { prompt: number; completion: number };
}

export const formatSummaryLine = ({ agentLabel, metrics, statusNote, usageSnapshot }: SummaryLineOptions): string => {
  const segments: string[] = [];
  const duration = formatDuration(metrics?.durationMs);
  if (typeof duration === 'string' && duration.length > 0) segments.push(`duration **${escapeMarkdown(duration)}**`);
  const normalizedCost = typeof metrics?.costUsd === 'number' && Number.isFinite(metrics.costUsd)
    ? `cost **$${metrics.costUsd.toFixed(4)}**`
    : undefined;
  if (normalizedCost !== undefined) segments.push(normalizedCost);
  const agents = typeof metrics?.agentsRun === 'number' && Number.isFinite(metrics.agentsRun)
    ? `agents ${String(metrics.agentsRun)}`
    : undefined;
  if (agents !== undefined) segments.push(agents);
  const tools = typeof metrics?.toolsRun === 'number' && Number.isFinite(metrics.toolsRun)
    ? `tools ${String(metrics.toolsRun)}`
    : undefined;
  if (tools !== undefined) segments.push(tools);
  const tokenMetrics: ProgressMetrics | undefined = metrics ?? (usageSnapshot !== undefined
    ? { tokensIn: usageSnapshot.prompt, tokensOut: usageSnapshot.completion }
    : undefined);
  const tokens = tokenMetrics !== undefined ? formatTokenSegments(tokenMetrics) : undefined;
  if (tokens !== undefined) segments.push(tokens);
  const safeStatus = typeof statusNote === 'string' && statusNote.trim().length > 0
    ? escapeMarkdown(statusNote.trim())
    : undefined;
  let summary = `SUMMARY: ${agentLabel}`;
  if (safeStatus !== undefined) summary += `, ${safeStatus}`;
  if (segments.length > 0) {
    summary += `, ${segments.join(', ')}`;
  } else if (safeStatus === undefined) {
    summary += ', completed';
  }
  return summary;
};

const sanitizeAgentLabelSegment = (raw: string): string => {
  if (raw.length === 0) return raw;
  const normalized = raw.replace(/\\/gu, '/');
  const segments = normalized.split('/').filter((segment) => segment.length > 0);
  const candidate = segments.length > 0 ? segments[segments.length - 1] : normalized;
  const trimmed = candidate.endsWith('.ai') ? candidate.slice(0, -3) : candidate;
  return trimmed.trim();
};

export const resolveAgentHeadingLabel = (agent: AgentMetadata): string => {
  if (typeof agent.toolName === 'string' && agent.toolName.trim().length > 0) {
    const preferred = sanitizeAgentLabelSegment(agent.toolName.trim());
    if (preferred.length > 0) return preferred;
  }
  const fallback = sanitizeAgentLabelSegment(agent.id);
  return fallback.length > 0 ? fallback : agent.id;
};
