import { isPlainObject } from '../utils.js';

const sortObject = (value: Record<string, unknown>): Record<string, unknown> => (
  Object.keys(value)
    .sort((a, b) => a.localeCompare(b))
    .reduce<Record<string, unknown>>((acc, key) => {
      acc[key] = value[key];
      return acc;
    }, {})
);

export const stableStringify = (value: unknown): string => {
  try {
    const replacer = (_key: string, val: unknown): unknown => (isPlainObject(val) ? sortObject(val) : val);
    return JSON.stringify(value, replacer);
  } catch {
    try {
      return JSON.stringify(String(value));
    } catch {
      return '"[unserializable]"';
    }
  }
};
