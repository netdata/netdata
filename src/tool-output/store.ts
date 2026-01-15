import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

import type { ToolOutputEntry, ToolOutputStats, ToolOutputTarget } from './types.js';

export class ToolOutputStore {
  private readonly entries = new Map<string, ToolOutputEntry>();
  private readonly runRootDir: string;
  private readonly sessionDirName: string;
  private readonly sessionDir: string;

  constructor(runRootDir: string, sessionId: string) {
    this.runRootDir = runRootDir;
    this.sessionDirName = `session-${sessionId}`;
    this.sessionDir = path.join(this.runRootDir, this.sessionDirName);
    fs.mkdirSync(this.sessionDir, { recursive: true });
  }

  getRunRootDir(): string {
    return this.runRootDir;
  }

  getSessionDirName(): string {
    return this.sessionDirName;
  }

  getSessionDir(): string {
    return this.sessionDir;
  }

  hasEntries(): boolean {
    return this.entries.size > 0;
  }

  listHandles(): string[] {
    return Array.from(this.entries.keys());
  }

  getEntry(handle: string): ToolOutputEntry | undefined {
    return this.entries.get(handle);
  }

  async read(handle: string): Promise<{ entry: ToolOutputEntry; content: string } | undefined> {
    const entry = this.entries.get(handle);
    if (entry === undefined) return undefined;
    try {
      const content = await fs.promises.readFile(entry.path, 'utf8');
      return { entry, content };
    } catch {
      return undefined;
    }
  }

  async store(args: {
    toolName: string;
    toolArgsJson: string;
    content: string;
    stats: ToolOutputStats;
    sourceTarget?: ToolOutputTarget;
  }): Promise<ToolOutputEntry> {
    const fileId = crypto.randomUUID();
    const handle = path.posix.join(this.sessionDirName, fileId);
    const outPath = path.join(this.runRootDir, ...handle.split('/'));
    await fs.promises.writeFile(outPath, args.content, 'utf8');
    const entry: ToolOutputEntry = {
      handle,
      toolName: args.toolName,
      toolArgsJson: args.toolArgsJson,
      bytes: args.stats.bytes,
      lines: args.stats.lines,
      tokens: args.stats.tokens,
      createdAt: Date.now(),
      path: outPath,
      sourceTarget: args.sourceTarget,
    };
    this.entries.set(handle, entry);
    return entry;
  }

  async cleanup(): Promise<void> {
    this.entries.clear();
    try {
      await fs.promises.rm(this.sessionDir, { recursive: true, force: true });
    } catch {
      // ignore cleanup failures
    }
  }
}
