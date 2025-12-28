export type DurationInput = number | string | null | undefined;
export type CacheDurationInput = DurationInput;

const UNIT_TO_MS: Record<string, number> = {
  ms: 1,
  s: 1000,
  m: 60 * 1000,
  h: 60 * 60 * 1000,
  d: 24 * 60 * 60 * 1000,
  w: 7 * 24 * 60 * 60 * 1000,
  mo: 30 * 24 * 60 * 60 * 1000,
  y: 365 * 24 * 60 * 60 * 1000,
};

export const parseDurationMs = (value: DurationInput, opts?: { allowOff?: boolean }): number | undefined => {
  if (value === undefined || value === null) return undefined;
  if (typeof value === 'number') {
    if (!Number.isFinite(value) || value < 0) return undefined;
    return Math.trunc(value);
  }
  if (typeof value !== 'string') return undefined;
  const raw = value.trim();
  if (raw.length === 0) return undefined;
  const lowered = raw.toLowerCase();
  if (lowered === 'off') return opts?.allowOff === true ? 0 : undefined;
  const asNumber = Number.parseFloat(lowered);
  if (Number.isFinite(asNumber) && /^\d+(\.\d+)?$/.test(lowered)) {
    if (asNumber < 0) return undefined;
    return Math.trunc(asNumber);
  }
  const match = /^(\d+(?:\.\d+)?)(mo|ms|s|m|h|d|w|y)$/i.exec(lowered);
  if (match === null) return undefined;
  const amount = Number.parseFloat(match[1]);
  if (!Number.isFinite(amount) || amount < 0) return undefined;
  const unit = match[2].toLowerCase();
  const ms = amount * UNIT_TO_MS[unit];
  if (!Number.isFinite(ms) || ms < 0) return undefined;
  return Math.trunc(ms);
};

export const parseDurationMsStrict = (value: DurationInput, context: string, opts?: { allowOff?: boolean }): number => {
  const parsed = parseDurationMs(value, opts);
  if (parsed === undefined) {
    const allowOff = opts?.allowOff === true;
    const offHint = allowOff ? "'off', " : '';
    throw new Error(`${context} duration must be ${offHint}a millisecond number, or a duration like 5m/2h/1d`);
  }
  return parsed;
};

export const parseCacheDurationMs = (value: CacheDurationInput): number | undefined => (
  parseDurationMs(value, { allowOff: true })
);

export const parseCacheDurationMsStrict = (value: CacheDurationInput, context: string): number => {
  const parsed = parseDurationMs(value, { allowOff: true });
  if (parsed === undefined) {
    throw new Error(`${context} cache duration must be 'off', a millisecond number, or a duration like 5m/2h/1d`);
  }
  return parsed;
};
