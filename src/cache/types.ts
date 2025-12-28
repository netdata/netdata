export type CacheKind = 'agent' | 'tool';

export interface CacheEntryMetadata {
  kind: CacheKind;
  agentName?: string;
  toolName?: string;
  agentHash?: string;
  toolNamespace?: string;
  format?: string;
}

export interface CacheEntry {
  keyHash: string;
  payload: Buffer;
  payloadBytes: number;
  createdAt: number;
  expiresAt: number;
  metadata: CacheEntryMetadata;
}

export interface CacheLookupResult extends CacheEntry {
  ageMs: number;
}

export interface CacheStore {
  get: (keyHash: string, nowMs: number) => Promise<CacheLookupResult | undefined>;
  set: (entry: CacheEntry) => Promise<void>;
}

export interface CacheConfig {
  backend?: 'sqlite' | 'redis';
  sqlite?: {
    path?: string;
  };
  redis?: {
    url?: string;
    username?: string;
    password?: string;
    database?: number;
    keyPrefix?: string;
  };
  maxEntries?: number;
}
