import { createClient, type RedisClientType } from 'redis';

import type { CacheEntry, CacheLookupResult, CacheStore } from './types.js';

import { warn } from '../utils.js';

interface RedisConfig {
  url?: string;
  username?: string;
  password?: string;
  database?: number;
  keyPrefix?: string;
}

interface RedisPayload {
  payload: string;
  payloadBytes: number;
  createdAt: number;
  expiresAt: number;
  metadata: CacheEntry['metadata'];
}

export class RedisCacheStore implements CacheStore {
  private readonly client: RedisClientType;
  private readonly keyPrefix: string;

  constructor(config: RedisConfig) {
    const url = typeof config.url === 'string' && config.url.length > 0 ? config.url : undefined;
    this.client = createClient({
      url,
      username: config.username,
      password: config.password,
      database: config.database,
    });
    this.keyPrefix = typeof config.keyPrefix === 'string' ? config.keyPrefix : 'ai-agent:cache:';
    this.client.on('error', (error) => {
      const message = error instanceof Error ? error.message : String(error);
      warn(`cache redis error: ${message}`);
    });
    void this.client.connect().catch((error: unknown) => {
      const message = error instanceof Error ? error.message : String(error);
      warn(`cache redis connect failed: ${message}`);
    });
  }

  private buildKey(keyHash: string): string {
    return `${this.keyPrefix}${keyHash}`;
  }

  async get(keyHash: string, nowMs: number): Promise<CacheLookupResult | undefined> {
    const key = this.buildKey(keyHash);
    const raw = await this.client.get(key);
    if (raw === null) return undefined;
    let parsed: RedisPayload | undefined;
    try {
      parsed = JSON.parse(raw) as RedisPayload;
    } catch {
      return undefined;
    }
    if (typeof parsed.expiresAt !== 'number' || parsed.expiresAt <= nowMs) {
      return undefined;
    }
    const payloadBuf = Buffer.from(parsed.payload, 'base64');
    const ageMs = Math.max(0, nowMs - parsed.createdAt);
    return {
      keyHash,
      payload: payloadBuf,
      payloadBytes: parsed.payloadBytes,
      createdAt: parsed.createdAt,
      expiresAt: parsed.expiresAt,
      metadata: parsed.metadata,
      ageMs,
    };
  }

  async set(entry: CacheEntry): Promise<void> {
    const ttlMs = Math.max(0, entry.expiresAt - entry.createdAt);
    if (ttlMs <= 0) return;
    const payload: RedisPayload = {
      payload: entry.payload.toString('base64'),
      payloadBytes: entry.payloadBytes,
      createdAt: entry.createdAt,
      expiresAt: entry.expiresAt,
      metadata: entry.metadata,
    };
    const key = this.buildKey(entry.keyHash);
    await this.client.set(key, JSON.stringify(payload), { PX: ttlMs });
  }
}
