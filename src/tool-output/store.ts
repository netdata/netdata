import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

import type { ToolOutputEntry, ToolOutputStats, ToolOutputTarget } from './types.js';

export class ToolOutputStore {
  private readonly entries = new Map<string, ToolOutputEntry>();
  private readonly rootDir: string;

  constructor(baseDir: string, sessionId: string) {
    this.rootDir = path.join(baseDir, `ai-agent-${sessionId}`);
    fs.mkdirSync(this.rootDir, { recursive: true });
  }

  getRootDir(): string {
    return this.rootDir;
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
    const handle = crypto.randomUUID();
    const outPath = path.join(this.rootDir, handle);
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
      await fs.promises.rm(this.rootDir, { recursive: true, force: true });
    } catch {
      // ignore cleanup failures
    }
  }
}
