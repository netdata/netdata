import type { EmbedMetricsConfig } from '../types.js';

type MetricLabels = Record<string, string>;

const escapeLabelValue = (value: string): string => value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n');

const formatLabels = (labels: MetricLabels): string => {
  const entries = Object.entries(labels)
    .map(([key, value]) => `${key}="${escapeLabelValue(value)}"`);
  if (entries.length === 0) return '';
  return `{${entries.join(',')}}`;
};

const formatNumber = (value: number): string => String(value);

export class EmbedMetrics {
  private readonly enabled: boolean;
  private readonly path: string;
  private sessionsActive = 0;
  private readonly requestsTotal = new Map<string, number>();
  private readonly errorsTotal = new Map<string, number>();
  private reportChunksTotal = 0;
  private sessionDurationSumSeconds = 0;
  private sessionDurationCount = 0;

  public constructor(config?: EmbedMetricsConfig) {
    this.enabled = config?.enabled ?? true;
    this.path = typeof config?.path === 'string' && config.path.length > 0 ? config.path : '/metrics';
  }

  public isEnabled(): boolean {
    return this.enabled;
  }

  public getPath(): string {
    return this.path;
  }

  public recordRequest(agent: string, origin: string, status: string): void {
    const key = `${agent}|${origin}|${status}`;
    this.requestsTotal.set(key, (this.requestsTotal.get(key) ?? 0) + 1);
  }

  public recordError(code: string): void {
    this.errorsTotal.set(code, (this.errorsTotal.get(code) ?? 0) + 1);
  }

  public recordReportChunk(): void {
    this.reportChunksTotal += 1;
  }

  public recordSessionStart(): void {
    this.sessionsActive += 1;
  }

  public recordSessionEnd(durationMs: number): void {
    this.sessionsActive = Math.max(0, this.sessionsActive - 1);
    const durationSeconds = Math.max(0, durationMs / 1000);
    this.sessionDurationSumSeconds += durationSeconds;
    this.sessionDurationCount += 1;
  }

  public renderPrometheus(): string {
    const lines: string[] = [];
    lines.push('# HELP embed_requests_total Total number of embed requests');
    lines.push('# TYPE embed_requests_total counter');
    this.requestsTotal.forEach((value, key) => {
      const [agent, origin, status] = key.split('|');
      lines.push(`embed_requests_total${formatLabels({ agent, origin, status })} ${formatNumber(value)}`);
    });

    lines.push('# HELP embed_sessions_active Current number of active embed sessions');
    lines.push('# TYPE embed_sessions_active gauge');
    lines.push(`embed_sessions_active ${formatNumber(this.sessionsActive)}`);

    lines.push('# HELP embed_session_duration_seconds Total duration of embed sessions in seconds');
    lines.push('# TYPE embed_session_duration_seconds summary');
    lines.push(`embed_session_duration_seconds_sum ${formatNumber(this.sessionDurationSumSeconds)}`);
    lines.push(`embed_session_duration_seconds_count ${formatNumber(this.sessionDurationCount)}`);

    lines.push('# HELP embed_report_chunks_total Total number of streamed report chunks');
    lines.push('# TYPE embed_report_chunks_total counter');
    lines.push(`embed_report_chunks_total ${formatNumber(this.reportChunksTotal)}`);

    lines.push('# HELP embed_errors_total Total number of embed errors');
    lines.push('# TYPE embed_errors_total counter');
    this.errorsTotal.forEach((value, code) => {
      lines.push(`embed_errors_total${formatLabels({ code })} ${formatNumber(value)}`);
    });

    return `${lines.join('\n')}\n`;
  }
}
