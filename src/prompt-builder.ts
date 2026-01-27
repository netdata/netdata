export function buildPromptVars(): Record<string, string> {
  const pad2 = (n: number): string => (n < 10 ? `0${String(n)}` : String(n));
  const formatRFC3339UTC = (d: Date): string => {
    const y = d.getUTCFullYear();
    const m = pad2(d.getUTCMonth() + 1);
    const da = pad2(d.getUTCDate());
    const hh = pad2(d.getUTCHours());
    const mm = pad2(d.getUTCMinutes());
    const ss = pad2(d.getUTCSeconds());
    return `${String(y)}-${m}-${da}T${hh}:${mm}:${ss}+00:00`;
  };
  // TODO: Support per-session timezone via headend user detection
  const timezone = 'UTC';
  const now = new Date();
  return {
    DATETIME: formatRFC3339UTC(now),
    TIMESTAMP: String(Math.floor(now.getTime() / 1000)),
    DAY: now.toLocaleDateString('en-US', { weekday: 'long', timeZone: timezone }),
    TIMEZONE: timezone,
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

