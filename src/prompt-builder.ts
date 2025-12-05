export function buildPromptVars(): Record<string, string> {
  const pad2 = (n: number): string => (n < 10 ? `0${String(n)}` : String(n));
  const formatRFC3339Local = (d: Date): string => {
    const y = d.getFullYear();
    const m = pad2(d.getMonth() + 1);
    const da = pad2(d.getDate());
    const hh = pad2(d.getHours());
    const mm = pad2(d.getMinutes());
    const ss = pad2(d.getSeconds());
    const tzMin = -d.getTimezoneOffset();
    const sign = tzMin >= 0 ? '+' : '-';
    const abs = Math.abs(tzMin);
    const tzh = pad2(Math.floor(abs / 60));
    const tzm = pad2(abs % 60);
    return `${String(y)}-${m}-${da}T${hh}:${mm}:${ss}${sign}${tzh}:${tzm}`;
  };
  const detectTimezone = (): string => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return process.env.TZ ?? 'UTC'; } };
  const now = new Date();
  return {
    DATETIME: formatRFC3339Local(now),
    TIMESTAMP: String(Math.floor(now.getTime() / 1000)),
    DAY: now.toLocaleDateString(undefined, { weekday: 'long' }),
    TIMEZONE: detectTimezone(),
  };
}

export function applyFormat(text: string, fmtDesc: string): string {
  return text.replace(/\$\{FORMAT\}|\{\{FORMAT\}\}/g, fmtDesc);
}

export function expandVars(text: string, vars: Record<string, string>): string {
  const replace = (s: string, re: RegExp) => s.replace(re, (_m, name: string) => (name in vars ? vars[name] : _m));
  let out = text;
  out = replace(out, /\$\{([A-Z_]+)\}/g);
  out = replace(out, /\{\{([A-Z_]+)\}\}/g);
  return out;
}

