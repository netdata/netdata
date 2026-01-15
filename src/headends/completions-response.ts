import { isPlainObject } from '../utils.js';

const getString = (value: unknown): string | undefined => (
  typeof value === 'string' && value.length > 0 ? value : undefined
);

export const resolveFinalReportContent = (output: string, finalReport: unknown): string => {
  if (!isPlainObject(finalReport)) return output;
  const format = typeof finalReport.format === 'string' ? finalReport.format : undefined;
  if (format === 'json' && finalReport.content_json !== undefined) {
    try { return JSON.stringify(finalReport.content_json); } catch { /* ignore */ }
  }
  const content = getString(finalReport.content);
  return content ?? output;
};
