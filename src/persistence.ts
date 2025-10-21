import fs from 'node:fs';
import path from 'node:path';
import { promisify } from 'node:util';
import { gzip as gzipCallback } from 'node:zlib';

import type { AIAgentCallbacks } from './types.js';

import { warn } from './utils.js';

const gzip = promisify(gzipCallback);

export interface PersistenceConfig {
  sessionsDir?: string;
  billingFile?: string;
}

type SnapshotPayload = Parameters<NonNullable<AIAgentCallbacks['onSessionSnapshot']>>[0];
type LedgerPayload = Parameters<NonNullable<AIAgentCallbacks['onAccountingFlush']>>[0];

export function createPersistenceCallbacks(config?: PersistenceConfig): Pick<AIAgentCallbacks, 'onSessionSnapshot' | 'onAccountingFlush'> {
  const callbacks: Pick<AIAgentCallbacks, 'onSessionSnapshot' | 'onAccountingFlush'> = {};
  if (typeof config?.sessionsDir === 'string' && config.sessionsDir.trim().length > 0) {
    const dir = path.resolve(config.sessionsDir);
    callbacks.onSessionSnapshot = async (payload: SnapshotPayload) => {
      try {
        await fs.promises.mkdir(dir, { recursive: true });
        const json = JSON.stringify({
          version: payload.snapshot.version,
          reason: payload.reason,
          opTree: payload.snapshot.opTree,
        });
        const gz = await gzip(Buffer.from(json, 'utf8'));
        const filePath = path.join(dir, `${payload.originId}.json.gz`);
        const tmp = `${filePath}.tmp-${String(process.pid)}-${String(Date.now())}`;
        await fs.promises.writeFile(tmp, gz);
        await fs.promises.rename(tmp, filePath);
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`session snapshot persistence failed: ${message}`);
      }
    };
  }
  if (typeof config?.billingFile === 'string' && config.billingFile.trim().length > 0) {
    const file = path.resolve(config.billingFile);
    callbacks.onAccountingFlush = async (payload: LedgerPayload) => {
      try {
        if (payload.entries.length === 0) return;
        const dir = path.dirname(file);
        await fs.promises.mkdir(dir, { recursive: true });
        const lines = payload.entries.map((entry) => JSON.stringify(entry));
        await fs.promises.appendFile(file, `${lines.join('\n')}\n`, 'utf8');
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`accounting persistence failed: ${message}`);
      }
    };
  }
  return callbacks;
}

export function mergeCallbacksWithPersistence(callbacks: AIAgentCallbacks | undefined, config?: PersistenceConfig): AIAgentCallbacks | undefined {
  const persistence = createPersistenceCallbacks(config);
  const needSnapshot = persistence.onSessionSnapshot !== undefined && callbacks?.onSessionSnapshot === undefined;
  const needLedger = persistence.onAccountingFlush !== undefined && callbacks?.onAccountingFlush === undefined;
  if (!needSnapshot && !needLedger) return callbacks;

  const base = callbacks ?? {};
  const merged: AIAgentCallbacks = { ...base };

  if (needSnapshot && persistence.onSessionSnapshot !== undefined) {
    const persistFn = persistence.onSessionSnapshot;
    merged.onSessionSnapshot = async (payload) => {
      try { await persistFn(payload); } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`session snapshot persistence failed: ${message}`);
      }
      await base.onSessionSnapshot?.(payload);
    };
  }

  if (needLedger && persistence.onAccountingFlush !== undefined) {
    const persistFn = persistence.onAccountingFlush;
    merged.onAccountingFlush = async (payload) => {
      try { await persistFn(payload); } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`accounting persistence failed: ${message}`);
      }
      await base.onAccountingFlush?.(payload);
    };
  }

  return merged;
}
