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
