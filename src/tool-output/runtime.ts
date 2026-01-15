import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

import type { ShutdownController } from '../shutdown-controller.js';

import { warn } from '../utils.js';

const RUN_HASH_LENGTH = 12;

const buildRunHash = (): string => {
  try {
    const seed = [process.pid, Date.now(), crypto.randomUUID()].map(String).join(':');
    return crypto.createHash('sha256').update(seed).digest('hex').slice(0, RUN_HASH_LENGTH);
  } catch {
    return crypto.randomUUID().replace(/-/g, '').slice(0, RUN_HASH_LENGTH);
  }
};

const RUN_HASH = buildRunHash();
let runRootDir: string | undefined;
let cleanupRegistered = false;

const runBaseDir = '/tmp';

const ensureRunRoot = (baseDir: string | undefined): string => {
  if (runRootDir !== undefined) {
    if (typeof baseDir === 'string' && baseDir.trim().length > 0) {
      const resolved = path.resolve(baseDir);
      if (resolved !== runBaseDir) {
        warn(`tool_output base dir override ignored (using ${runBaseDir}, requested ${resolved}).`);
      }
    }
    return runRootDir;
  }
  if (typeof baseDir === 'string' && baseDir.trim().length > 0) {
    const resolved = path.resolve(baseDir);
    if (resolved !== runBaseDir) {
      warn(`tool_output base dir override ignored (using ${runBaseDir}, requested ${resolved}).`);
    }
  }
  runRootDir = path.join(runBaseDir, `ai-agent-${RUN_HASH}`);
  try {
    fs.mkdirSync(runRootDir, { recursive: true });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`Failed to create tool_output run directory at ${runRootDir}: ${message}`);
  }
  return runRootDir;
};

const cleanupRunRootSync = (): void => {
  if (runRootDir === undefined) return;
  try {
    fs.rmSync(runRootDir, { recursive: true, force: true });
  } catch {
    // ignore cleanup errors
  }
};

const registerProcessCleanup = (): void => {
  if (cleanupRegistered) return;
  cleanupRegistered = true;
  try {
    process.once('exit', () => {
      cleanupRunRootSync();
    });
  } catch {
    // ignore process handler failures
  }
};

export function ensureToolOutputRunRoot(baseDir?: string): string {
  const root = ensureRunRoot(baseDir);
  registerProcessCleanup();
  return root;
}

export function ensureToolOutputSessionDir(sessionId: string, baseDir?: string): {
  runRootDir: string;
  sessionDirName: string;
  sessionDir: string;
} {
  const root = ensureToolOutputRunRoot(baseDir);
  const sessionDirName = `session-${sessionId}`;
  const sessionDir = path.join(root, sessionDirName);
  try {
    fs.mkdirSync(sessionDir, { recursive: true });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`Failed to create tool_output session directory at ${sessionDir}: ${message}`);
  }
  return { runRootDir: root, sessionDirName, sessionDir };
}

export async function cleanupToolOutputSessionDir(sessionDir: string | undefined): Promise<void> {
  if (sessionDir === undefined || sessionDir.length === 0) return;
  try {
    await fs.promises.rm(sessionDir, { recursive: true, force: true });
  } catch {
    // ignore cleanup errors
  }
}

export async function cleanupToolOutputRunRoot(): Promise<void> {
  if (runRootDir === undefined) return;
  try {
    await fs.promises.rm(runRootDir, { recursive: true, force: true });
  } catch {
    // ignore cleanup errors
  }
}

export function registerToolOutputRunRootCleanup(
  shutdownController?: Pick<ShutdownController, 'register'>,
  baseDir?: string,
): void {
  ensureToolOutputRunRoot(baseDir ?? runBaseDir);
  if (shutdownController !== undefined) {
    shutdownController.register('tool-output-run-root', async () => {
      await cleanupToolOutputRunRoot();
    });
  }
  registerProcessCleanup();
}
