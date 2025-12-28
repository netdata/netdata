import os from 'node:os';
import path from 'node:path';

import type { CacheConfig } from './types.js';

import { warn } from '../utils.js';

import { RedisCacheStore } from './redis-store.js';
import { ResponseCache } from './response-cache.js';
import { SQLiteCacheStore } from './sqlite-store.js';
import { stableStringify } from './stable-stringify.js';

const DEFAULT_MAX_ENTRIES = 5000;

const resolveDefaultSqlitePath = (): string => {
  const home = os.homedir();
  const baseDir = home.length > 0 ? path.join(home, '.ai-agent') : path.join(process.cwd(), '.ai-agent');
  return path.join(baseDir, 'cache.db');
};

const normalizeConfig = (config?: CacheConfig): CacheConfig => {
  const backend = config?.backend ?? 'sqlite';
  const maxEntriesRaw = config?.maxEntries;
  const maxEntries = typeof maxEntriesRaw === 'number' && Number.isFinite(maxEntriesRaw) && maxEntriesRaw > 0
    ? Math.trunc(maxEntriesRaw)
    : DEFAULT_MAX_ENTRIES;
  const sqlitePath = config?.sqlite?.path ?? resolveDefaultSqlitePath();
  const sqlite = { path: sqlitePath };
  const redis = config?.redis ?? {};
  return { backend, maxEntries, sqlite, redis };
};

let cachedInstance: ResponseCache | undefined;
let cachedConfigKey: string | undefined;
let cachedUnavailableKey: string | undefined;

export const getResponseCache = (config?: CacheConfig): ResponseCache | undefined => {
  if (config === undefined) return undefined;
  const normalized = normalizeConfig(config);
  const configKey = stableStringify(normalized);
  if (cachedUnavailableKey === configKey) return undefined;
  if (cachedInstance !== undefined && cachedConfigKey === configKey) {
    return cachedInstance;
  }
  const store = (() => {
    if (normalized.backend === 'redis') {
      return new RedisCacheStore(normalized.redis ?? {});
    }
    try {
      return new SQLiteCacheStore({ path: normalized.sqlite?.path ?? resolveDefaultSqlitePath(), maxEntries: normalized.maxEntries ?? DEFAULT_MAX_ENTRIES });
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      warn(`cache disabled: sqlite backend unavailable (${message})`);
      return undefined;
    }
  })();
  if (store === undefined) {
    cachedUnavailableKey = configKey;
    return undefined;
  }
  cachedUnavailableKey = undefined;
  cachedInstance = new ResponseCache(store);
  cachedConfigKey = configKey;
  return cachedInstance;
};

export const getDefaultCacheMaxEntries = (): number => DEFAULT_MAX_ENTRIES;
