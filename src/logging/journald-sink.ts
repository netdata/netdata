import { spawn, type ChildProcess } from 'node:child_process';
import fs from 'node:fs';

import type { StructuredLogEvent } from './structured-log-event.js';

const JOURNAL_SOCKET_PATH = '/run/systemd/journal/socket';
const SYSTEMD_CAT_PATHS = [
  '/usr/sbin/systemd-cat-native',
  '/usr/bin/systemd-cat-native',
  '/sbin/systemd-cat-native',
  '/bin/systemd-cat-native',
  '/opt/netdata/usr/bin/systemd-cat-native',
];

let cachedSystemdCatPath: string | undefined;

function findSystemdCatNative(): string | undefined {
  if (cachedSystemdCatPath !== undefined) return cachedSystemdCatPath;

  const found = SYSTEMD_CAT_PATHS.find((path) => {
    try {
      const stats = fs.statSync(path);
      if (!stats.isFile()) return false;
      fs.accessSync(path, fs.constants.X_OK);
      return true;
    } catch {
      return false;
    }
  });

  if (found !== undefined) {
    cachedSystemdCatPath = found;
  }

  return found;
}

export function isJournaldAvailable(): boolean {
  if (process.env.JOURNAL_STREAM === undefined) return false;
  try {
    const stats = fs.statSync(JOURNAL_SOCKET_PATH);
    return stats.isSocket();
  } catch {
    return false;
  }
}

type DisableListener = () => void;

export interface JournaldEmitter {
  emit: (event: StructuredLogEvent) => boolean;
  onDisable: (listener: DisableListener) => () => void;
  isDisabled: () => boolean;
}

let sharedSink: SharedJournaldSink | undefined;

export function acquireJournaldSink(): JournaldEmitter | undefined {
  if (!isJournaldAvailable()) return undefined;
  if (sharedSink === undefined) {
    const candidate = new SharedJournaldSink();
    if (candidate.isDisabled()) return undefined;
    sharedSink = candidate;
  }
  return sharedSink;
}

class SharedJournaldSink implements JournaldEmitter {
  private child?: ChildProcess;
  private stdin?: NodeJS.WritableStream;
  private disabled = false;
  private restartAttempts = 0;
  private readonly disableListeners = new Set<DisableListener>();

  constructor() {
    const failure = this.spawnHelper();
    if (failure !== undefined) {
      this.disable(failure);
    }
  }

  emit(event: StructuredLogEvent): boolean {
    if (this.disabled) return false;
    const writer = this.stdin;
    if (writer === undefined) return false;
    const payload = this.formatPayload(event);
    try {
      const flushed = writer.write(payload, 'utf8');
      if (!flushed) writer.once('drain', () => undefined);
      return true;
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error);
      this.disable(`failed to write to systemd-cat-native: ${reason}`);
      return false;
    }
  }

  onDisable(listener: DisableListener): () => void {
    if (this.disabled) {
      try { listener(); } catch { /* ignore */ }
      return () => undefined;
    }
    this.disableListeners.add(listener);
    return () => {
      this.disableListeners.delete(listener);
    };
  }

  isDisabled(): boolean {
    return this.disabled;
  }

  private spawnHelper(): string | undefined {
    if (!isJournaldAvailable()) {
      return 'systemd journald unavailable';
    }

    const binaryPath = findSystemdCatNative();
    if (binaryPath === undefined) {
      return 'systemd-cat-native not found in standard locations';
    }

    try {
      const child = spawn(binaryPath, [], {
        stdio: ['pipe', 'ignore', 'ignore'],
      });

      // Attach error handlers before any other operations
      child.once('error', (error: Error) => {
        const reason = `failed to run systemd-cat-native: ${error.message}`;
        this.handleHelperFailure(reason);
      });

      child.once('exit', (code, signal) => {
        const result = code !== null ? `exit code ${String(code)}` : `signal ${String(signal)}`;
        this.handleHelperExit(`systemd-cat-native exited (${result})`);
      });

      const stdin = child.stdin;
      stdin.once('error', (err: Error) => {
        const message = err instanceof Error ? err.message : String(err);
        this.disable(`systemd-cat-native stdin error: ${message}`);
      });

      this.child = child;
      this.stdin = stdin;
      this.restartAttempts = 0;
      return undefined;
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error);
      return `failed to spawn systemd-cat-native: ${reason}`;
    }
  }

  private handleHelperExit(reason: string): void {
    if (this.disabled) return;
    this.cleanupChild();
    this.attemptRestart(reason);
  }

  private handleHelperFailure(reason: string): void {
    if (this.disabled) return;
    this.cleanupChild();
    this.attemptRestart(reason);
  }

  private cleanupChild(): void {
    if (this.stdin !== undefined) {
      try { this.stdin.removeAllListeners('error'); } catch { /* ignore */ }
      try { this.stdin.end(); } catch { /* ignore */ }
    }
    if (this.child !== undefined) {
      try {
        this.child.removeAllListeners('error');
        this.child.removeAllListeners('exit');
        this.child.kill();
      } catch {
        /* ignore */
      }
    }
    this.child = undefined;
    this.stdin = undefined;
  }

  private disable(reason: string): void {
    if (this.disabled) return;
    this.disabled = true;
    this.cleanupChild();
    try {
      process.stderr.write(`journald sink disabled: ${reason}; falling back to logfmt stderr\n`);
    } catch {
      /* ignore */
    }
    this.disableListeners.forEach((listener) => {
      try { listener(); } catch { /* ignore */ }
    });
    this.disableListeners.clear();
    if (sharedSink === this) {
      sharedSink = undefined;
    }
  }

  private attemptRestart(reason: string): void {
    if (this.restartAttempts >= 1) {
      this.disable(`${reason}; restart exhausted`);
      return;
    }
    this.restartAttempts += 1;
    const restartFailure = this.spawnHelper();
    if (restartFailure !== undefined) {
      this.disable(`${reason}; restart failed: ${restartFailure}`);
    }
  }

  private formatPayload(event: StructuredLogEvent): string {
    const lines: string[] = [];
    lines.push(`MESSAGE=${sanitizeMessage(event.message)}`);
    lines.push(`PRIORITY=${String(event.priority)}`);
    lines.push('SYSLOG_IDENTIFIER=ai-agent');
    if (event.messageId !== undefined) lines.push(`MESSAGE_ID=${event.messageId}`);
    lines.push(`AI_SEVERITY=${event.severity}`);
    lines.push(`AI_TIMESTAMP=${event.isoTimestamp}`);
    if (typeof event.agentId === 'string' && event.agentId.length > 0) lines.push(`AI_AGENT=${event.agentId}`);
    if (typeof event.callPath === 'string' && event.callPath.length > 0) lines.push(`AI_CALL_PATH=${event.callPath}`);
    if (typeof event.remoteIdentifier === 'string' && event.remoteIdentifier.length > 0) lines.push(`AI_REMOTE=${event.remoteIdentifier}`);
    if (typeof event.provider === 'string' && event.provider.length > 0) lines.push(`AI_PROVIDER=${event.provider}`);
    if (typeof event.model === 'string' && event.model.length > 0) lines.push(`AI_MODEL=${event.model}`);
    if (typeof event.toolKind === 'string' && event.toolKind.length > 0) lines.push(`AI_TOOL_KIND=${event.toolKind}`);
    if (typeof event.toolProvider === 'string' && event.toolProvider.length > 0) lines.push(`AI_TOOL_PROVIDER=${event.toolProvider}`);
    if (typeof event.tool === 'string' && event.tool.length > 0) lines.push(`AI_TOOL=${event.tool}`);
    if (typeof event.headendId === 'string' && event.headendId.length > 0) lines.push(`AI_HEADEND=${event.headendId}`);
    lines.push(`AI_DIRECTION=${event.direction}`);
    lines.push(`AI_TYPE=${event.type}`);
    lines.push(`AI_TURN=${String(event.turn)}`);
    lines.push(`AI_SUBTURN=${String(event.subturn)}`);
    if (typeof event.txnId === 'string' && event.txnId.length > 0) lines.push(`AI_TXN_ID=${event.txnId}`);
    if (typeof event.parentTxnId === 'string' && event.parentTxnId.length > 0) lines.push(`AI_PARENT_TXN_ID=${event.parentTxnId}`);
    if (typeof event.originTxnId === 'string' && event.originTxnId.length > 0) lines.push(`AI_ORIGIN_TXN_ID=${event.originTxnId}`);
    Object.entries(event.labels).forEach(([key, value]) => {
      const upper = key.toUpperCase();
      lines.push(`AI_LABEL_${upper}=${value}`);
    });
    if (typeof event.stack === 'string' && event.stack.length > 0) {
      lines.push(`AI_STACKTRACE=${sanitizeMessage(event.stack)}`);
    }
    return `${lines.join('\n')}\n\n`;
  }
}

function sanitizeMessage(message: string): string {
  return message.replace(/\n/g, '\\n');
}
