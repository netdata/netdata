import fs from 'node:fs';
import path from 'node:path';
import { gzipSync } from 'node:zlib';

import type { AccountingFlushPayload, AIAgentEventCallbacks, SessionSnapshotPayload } from './types.js';

import { warn } from './utils.js';

export interface PersistenceConfig {
  sessionsDir?: string;
  billingFile?: string;
}

/**
 * Returns default persistence configuration based on user's home directory.
 * Sessions: ~/.ai-agent/sessions/
 * Billing:  ~/.ai-agent/accounting.jsonl
 */
export function getDefaultPersistenceConfig(): PersistenceConfig {
  const home = process.env.HOME ?? process.env.USERPROFILE ?? '';
  if (home.length === 0) return {};
  const base = path.join(home, '.ai-agent');
  return {
    sessionsDir: path.join(base, 'sessions'),
    billingFile: path.join(base, 'accounting.jsonl'),
  };
}

/**
 * Merges user-provided persistence config with defaults.
 * User config takes precedence; defaults fill in missing values.
 */
export function resolvePeristenceConfig(config?: PersistenceConfig): PersistenceConfig {
  const defaults = getDefaultPersistenceConfig();
  return {
    sessionsDir: config?.sessionsDir ?? defaults.sessionsDir,
    billingFile: config?.billingFile ?? defaults.billingFile,
  };
}

interface PersistenceHandlers {
  handleSnapshot?: (payload: SessionSnapshotPayload) => void;
  handleAccountingFlush?: (payload: AccountingFlushPayload) => void;
}

const createPersistenceHandlers = (config?: PersistenceConfig): PersistenceHandlers => {
  const handlers: PersistenceHandlers = {};
  if (typeof config?.sessionsDir === 'string' && config.sessionsDir.trim().length > 0) {
    const dir = path.resolve(config.sessionsDir);
    handlers.handleSnapshot = (payload: SessionSnapshotPayload) => {
      try {
        fs.mkdirSync(dir, { recursive: true });
        const json = JSON.stringify({
          version: payload.snapshot.version,
          reason: payload.reason,
          opTree: payload.snapshot.opTree,
        });
        const gz = gzipSync(Buffer.from(json, 'utf8'));
        const filePath = path.join(dir, `${payload.originId}.json.gz`);
        const tmp = `${filePath}.tmp-${String(process.pid)}-${String(Date.now())}`;
        fs.writeFileSync(tmp, gz);
        fs.renameSync(tmp, filePath);
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`session snapshot persistence failed: ${message}`);
      }
    };
  }
  if (typeof config?.billingFile === 'string' && config.billingFile.trim().length > 0) {
    const file = path.resolve(config.billingFile);
    handlers.handleAccountingFlush = (payload: AccountingFlushPayload) => {
      try {
        if (payload.entries.length === 0) return;
        const dir = path.dirname(file);
        fs.mkdirSync(dir, { recursive: true });
        const lines = payload.entries.map((entry) => JSON.stringify(entry));
        fs.appendFileSync(file, `${lines.join('\n')}\n`, 'utf8');
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`accounting persistence failed: ${message}`);
      }
    };
  }
  return handlers;
};

const wrappedCallbacks = new WeakSet<AIAgentEventCallbacks>();

export function createPersistenceCallbacks(config?: PersistenceConfig): AIAgentEventCallbacks {
  const handlers = createPersistenceHandlers(config);
  if (handlers.handleSnapshot === undefined && handlers.handleAccountingFlush === undefined) {
    return {};
  }
  return {
    onEvent: (event) => {
      if (event.type === 'snapshot' && handlers.handleSnapshot !== undefined) {
        handlers.handleSnapshot(event.payload);
      }
      if (event.type === 'accounting_flush' && handlers.handleAccountingFlush !== undefined) {
        handlers.handleAccountingFlush(event.payload);
      }
    },
  };
}

export function mergeCallbacksWithPersistence(
  callbacks: AIAgentEventCallbacks | undefined,
  config?: PersistenceConfig,
): AIAgentEventCallbacks | undefined {
  const handlers = createPersistenceHandlers(config);
  if (handlers.handleSnapshot === undefined && handlers.handleAccountingFlush === undefined) {
    return callbacks;
  }

  if (callbacks === undefined) {
    return {
      onEvent: (event) => {
        if (event.type === 'snapshot' && handlers.handleSnapshot !== undefined) {
          handlers.handleSnapshot(event.payload);
        }
        if (event.type === 'accounting_flush' && handlers.handleAccountingFlush !== undefined) {
          handlers.handleAccountingFlush(event.payload);
        }
      },
    };
  }

  if (wrappedCallbacks.has(callbacks)) {
    return callbacks;
  }
  wrappedCallbacks.add(callbacks);

  const base = callbacks.onEvent;
  callbacks.onEvent = (event, meta) => {
    if (event.type === 'snapshot' && handlers.handleSnapshot !== undefined) {
      handlers.handleSnapshot(event.payload);
    }
    if (event.type === 'accounting_flush' && handlers.handleAccountingFlush !== undefined) {
      handlers.handleAccountingFlush(event.payload);
    }
    if (base !== undefined) {
      try {
        base(event, meta);
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        warn(`onEvent callback failed: ${message}`);
      }
    }
  };

  return callbacks;
}
