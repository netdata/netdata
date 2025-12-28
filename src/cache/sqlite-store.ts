import fs from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';

import type { CacheEntry, CacheLookupResult, CacheStore } from './types.js';
import type { Database as SqliteDatabase, Statement as SqliteStatement } from 'better-sqlite3';

type SqliteFactory = new (filename: string) => SqliteDatabase;

interface SqliteStoreOptions {
  path: string;
  maxEntries: number;
}

interface Row {
  key_hash: string;
  payload: Buffer;
  payload_bytes: number;
  kind: string;
  agent_name: string | null;
  tool_name: string | null;
  agent_hash: string | null;
  tool_namespace: string | null;
  format: string | null;
  created_at: number;
  expires_at: number;
}

interface CountRow {
  count: number;
}

let cachedFactory: SqliteFactory | undefined;

const loadSqliteFactory = (): SqliteFactory => {
  if (cachedFactory !== undefined) return cachedFactory;
  const require = createRequire(import.meta.url);
  const factory = require('better-sqlite3') as SqliteFactory;
  cachedFactory = factory;
  return factory;
};

export class SQLiteCacheStore implements CacheStore {
  private readonly db: SqliteDatabase;
  private readonly maxEntries: number;
  private readonly getStmt: SqliteStatement<[string, number], Row>;
  private readonly insertStmt: SqliteStatement<[Record<string, unknown>]>;
  private readonly deleteExpiredStmt: SqliteStatement<[number]>;
  private readonly countStmt: SqliteStatement<[], CountRow>;
  private readonly deleteOverflowStmt: SqliteStatement<[number]>;

  constructor(opts: SqliteStoreOptions) {
    const filePath = opts.path;
    this.maxEntries = Math.max(1, Math.trunc(opts.maxEntries));
    const dir = path.dirname(filePath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }
    const sqlite = loadSqliteFactory();
    this.db = new sqlite(filePath);
    this.db.pragma('journal_mode = WAL');
    this.db.pragma('synchronous = NORMAL');
    this.db.pragma('busy_timeout = 5000');
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS cache_entries (
        key_hash TEXT PRIMARY KEY,
        payload BLOB NOT NULL,
        payload_bytes INTEGER NOT NULL,
        kind TEXT NOT NULL,
        agent_name TEXT,
        tool_name TEXT,
        agent_hash TEXT,
        tool_namespace TEXT,
        format TEXT,
        created_at INTEGER NOT NULL,
        expires_at INTEGER NOT NULL
      );
      CREATE INDEX IF NOT EXISTS idx_cache_expires ON cache_entries(expires_at);
      CREATE INDEX IF NOT EXISTS idx_cache_created ON cache_entries(created_at);
    `);

    this.getStmt = this.db.prepare<[string, number], Row>(`
      SELECT key_hash, payload, payload_bytes, kind, agent_name, tool_name, agent_hash, tool_namespace, format, created_at, expires_at
      FROM cache_entries
      WHERE key_hash = ? AND expires_at > ?
      LIMIT 1
    `);
    this.insertStmt = this.db.prepare<[Record<string, unknown>]>(`
      INSERT OR REPLACE INTO cache_entries
      (key_hash, payload, payload_bytes, kind, agent_name, tool_name, agent_hash, tool_namespace, format, created_at, expires_at)
      VALUES (@key_hash, @payload, @payload_bytes, @kind, @agent_name, @tool_name, @agent_hash, @tool_namespace, @format, @created_at, @expires_at)
    `);
    this.deleteExpiredStmt = this.db.prepare<[number]>(`
      DELETE FROM cache_entries
      WHERE expires_at <= ?
    `);
    this.countStmt = this.db.prepare<[], CountRow>('SELECT COUNT(*) as count FROM cache_entries');
    this.deleteOverflowStmt = this.db.prepare<[number]>(`
      DELETE FROM cache_entries
      WHERE rowid IN (
        SELECT rowid
        FROM cache_entries
        ORDER BY created_at ASC
        LIMIT ?
      )
    `);
  }

  get(keyHash: string, nowMs: number): Promise<CacheLookupResult | undefined> {
    const row = this.getStmt.get(keyHash, nowMs);
    if (row === undefined) return Promise.resolve(undefined);
    const ageMs = Math.max(0, nowMs - row.created_at);
    const metadata = {
      kind: row.kind as CacheLookupResult['metadata']['kind'],
      agentName: row.agent_name ?? undefined,
      toolName: row.tool_name ?? undefined,
      agentHash: row.agent_hash ?? undefined,
      toolNamespace: row.tool_namespace ?? undefined,
      format: row.format ?? undefined,
    };
    return Promise.resolve({
      keyHash: row.key_hash,
      payload: row.payload,
      payloadBytes: row.payload_bytes,
      createdAt: row.created_at,
      expiresAt: row.expires_at,
      metadata,
      ageMs,
    });
  }

  set(entry: CacheEntry): Promise<void> {
    const payloadBytes = entry.payloadBytes;
    const record = {
      key_hash: entry.keyHash,
      payload: entry.payload,
      payload_bytes: payloadBytes,
      kind: entry.metadata.kind,
      agent_name: entry.metadata.agentName ?? null,
      tool_name: entry.metadata.toolName ?? null,
      agent_hash: entry.metadata.agentHash ?? null,
      tool_namespace: entry.metadata.toolNamespace ?? null,
      format: entry.metadata.format ?? null,
      created_at: entry.createdAt,
      expires_at: entry.expiresAt,
    };

    const nowMs = entry.createdAt;
    this.db.transaction(() => {
      this.insertStmt.run(record);
      this.deleteExpiredStmt.run(nowMs);
      const row = this.countStmt.get();
      const count = typeof row?.count === 'number' ? row.count : 0;
      const overflow = Math.max(0, count - this.maxEntries);
      if (overflow > 0) {
        this.deleteOverflowStmt.run(overflow);
      }
    })();
    return Promise.resolve();
  }
}
