import { gunzipSync, gzipSync } from 'node:zlib';

import type { CacheEntry, CacheEntryMetadata, CacheLookupResult, CacheStore } from './types.js';

import { sha256Hex } from './hash.js';
import { stableStringify } from './stable-stringify.js';

export interface CacheHit<T> {
  entry: CacheLookupResult;
  value: T;
}

const encodePayload = (value: unknown): { payload: Buffer; payloadBytes: number } => {
  const json = JSON.stringify(value);
  const compressed = gzipSync(json);
  return { payload: compressed, payloadBytes: compressed.byteLength };
};

const decodePayload = (payload: Buffer): unknown => {
  try {
    const decompressed = gunzipSync(payload);
    const json = decompressed.toString('utf8');
    return JSON.parse(json) as unknown;
  } catch {
    return undefined;
  }
};

export class ResponseCache {
  constructor(private readonly store: CacheStore) {}

  buildKeyHash(payload: unknown): string {
    const raw = stableStringify(payload);
    return sha256Hex(raw);
  }

  async get(payloadKey: unknown, ttlMs: number, nowMs: number): Promise<CacheHit<unknown> | undefined> {
    if (!Number.isFinite(ttlMs) || ttlMs <= 0) return undefined;
    const keyHash = this.buildKeyHash(payloadKey);
    const entry = await this.store.get(keyHash, nowMs);
    if (entry === undefined) return undefined;
    if (entry.ageMs > ttlMs) return undefined;
    const decoded = decodePayload(entry.payload);
    if (decoded === undefined) return undefined;
    return { entry, value: decoded };
  }

  async set(payloadKey: unknown, ttlMs: number, value: unknown, metadata: CacheEntryMetadata, nowMs: number): Promise<void> {
    if (!Number.isFinite(ttlMs) || ttlMs <= 0) return;
    const keyHash = this.buildKeyHash(payloadKey);
    const { payload, payloadBytes } = encodePayload(value);
    const entry: CacheEntry = {
      keyHash,
      payload,
      payloadBytes,
      createdAt: nowMs,
      expiresAt: nowMs + Math.trunc(ttlMs),
      metadata,
    };
    await this.store.set(entry);
  }
}
