import assert from 'node:assert/strict';
import * as childProcess from 'node:child_process';
import { EventEmitter } from 'node:events';
import fs from 'node:fs';

import type { StructuredLogEvent } from '../../logging/structured-log-event.js';
import type { ChildProcess } from 'node:child_process';

class FakeWritable extends EventEmitter {
  public readonly writes: string[] = [];
  private callIndex = 0;
  private backpressureCalls: Set<number>;

  constructor(backpressureCalls: number[] = []) {
    super();
    this.backpressureCalls = new Set(backpressureCalls);
  }

  write(chunk: string | Uint8Array, encoding?: unknown, _callback?: unknown): boolean {
    this.callIndex += 1;
    const resolvedEncoding: BufferEncoding = typeof encoding === 'string' ? encoding as BufferEncoding : 'utf8';
    const text = typeof chunk === 'string'
      ? chunk
      : Buffer.from(chunk).toString(resolvedEncoding);
    this.writes.push(text);
    return !this.backpressureCalls.has(this.callIndex);
  }

  end(): void {
    /* no-op */
  }

  destroy(): void {
    /* no-op */
  }

  cork(): void {
    /* no-op */
  }

  uncork(): void {
    /* no-op */
  }

  setDefaultEncoding(): this {
    return this;
  }

  writev(): boolean {
    return true;
  }

  reset(): void {
    this.callIndex = 0;
    this.writes.length = 0;
  }
}

class FakeChildProcess extends EventEmitter {
  public readonly stdin: FakeWritable;
  public readonly stdout: null = null;
  public readonly stderr: null = null;
  public readonly stdio: [FakeWritable, null, null];
  public readonly pid = 4242;

  constructor(stdin: FakeWritable) {
    super();
    this.stdin = stdin;
    this.stdio = [stdin, null, null];
  }

  kill(): boolean {
    return true;
  }

  disconnect(): void {
    /* no-op */
  }

  unref(): void {
    /* no-op */
  }
}

type SpawnFn = typeof childProcess.spawn;
type StatFn = typeof fs.statSync;
type AccessFn = typeof fs.accessSync;

let activeWritable: FakeWritable | undefined;

function installStubs(): () => void {
  const originalSpawn: SpawnFn = childProcess.spawn;
  const originalStatSync: StatFn = fs.statSync;
  const originalAccessSync: AccessFn = fs.accessSync;

  const mutableFs = fs as unknown as { statSync: StatFn; accessSync: AccessFn };
  const mutableChild = childProcess as unknown as { spawn: SpawnFn };

  mutableFs.statSync = ((path: fs.PathLike) => {
    if (typeof path === 'string') {
      if (path === '/run/systemd/journal/socket') {
        return {
          isSocket: () => true,
          isFile: () => false,
        } as fs.Stats;
      }
      if (path.endsWith('systemd-cat-native')) {
        return {
          isSocket: () => false,
          isFile: () => true,
        } as fs.Stats;
      }
    }
    return originalStatSync(path);
  }) as StatFn;

  mutableFs.accessSync = ((..._args) => undefined) as AccessFn;

  mutableChild.spawn = ((..._args) => {
    if (activeWritable === undefined) {
      throw new Error('FakeWritable not configured before spawn.');
    }
    return new FakeChildProcess(activeWritable) as unknown as ChildProcess;
  }) as SpawnFn;

  return () => {
    mutableChild.spawn = originalSpawn;
    mutableFs.statSync = originalStatSync;
    mutableFs.accessSync = originalAccessSync;
    activeWritable = undefined;
  };
}

function buildEvent(message: string): StructuredLogEvent {
  const now = Date.now();
  return {
    timestamp: now,
    isoTimestamp: new Date(now).toISOString(),
    severity: 'WRN',
    priority: 4,
    message,
    type: 'tool',
    direction: 'response',
    turn: 0,
    subturn: 0,
    remoteIdentifier: 'test:logger',
    labels: {},
  };
}

async function createSink(writer: FakeWritable) {
  activeWritable = writer;
  const module = await import('../../logging/journald-sink.js');
  module.__test.resetSharedSink();
  const sink = module.acquireJournaldSink();
  assert.ok(sink !== undefined, 'journald sink should be acquired');
  return { sink, module, writer };
}

async function testBuffersAndFlushes(): Promise<void> {
  const writer = new FakeWritable([1]);
  const { sink } = await createSink(writer);
  const first = buildEvent('first');
  const second = buildEvent('second');

  const firstResult = sink.emit(first);
  assert.equal(firstResult, true);
  assert.equal(writer.writes.length, 1, 'first write should be queued internally');

  const secondResult = sink.emit(second);
  assert.equal(secondResult, true);
  assert.equal(writer.writes.length, 1, 'second log should buffer while drain pending');

  writer.emit('drain');
  assert.equal(writer.writes.length, 2, 'buffered log should flush after drain');
  const hasDropSummary = writer.writes.some((entry) => entry.includes('dropped'));
  assert.equal(hasDropSummary, false, 'no messages should be dropped in happy path');
}

async function testDropSummary(): Promise<void> {
  const writer = new FakeWritable([1]);
  const { sink } = await createSink(writer);
  sink.emit(buildEvent('initial'));

  const stderrWrites: string[] = [];
  const mutableStderr = process.stderr as unknown as { write: typeof process.stderr.write };
  const originalStderrWrite = mutableStderr.write;
  mutableStderr.write = ((chunk: string | Uint8Array, encoding?: BufferEncoding | ((err?: Error) => void), callback?: (err?: Error) => void): boolean => {
    const resolvedEncoding: BufferEncoding = typeof encoding === 'string' ? encoding : 'utf8';
    const text = typeof chunk === 'string' ? chunk : Buffer.from(chunk).toString(resolvedEncoding);
    stderrWrites.push(text);
    const cb = typeof encoding === 'function' ? encoding : callback;
    if (typeof cb === 'function') {
      cb();
    }
    return true;
  }) as typeof process.stderr.write;

  let dropped = 0;
  try {
    // eslint-disable-next-line functional/no-loop-statements -- precise control over buffered emit iterations simplifies drop counting
    for (let index = 0; index < 120; index += 1) {
      const ok = sink.emit(buildEvent(`buffer-${String(index)}`));
      if (!ok) {
        dropped += 1;
      }
    }
    assert.equal(dropped, 20, '20 log entries should be dropped when buffer exceeds 100');

    writer.emit('drain');
  } finally {
    mutableStderr.write = originalStderrWrite;
  }
  const summary = writer.writes.find((entry) => entry.includes('dropped 20 log entries'));
  assert.ok(summary !== undefined, 'drop summary should be written after drain');
  const stderrSummary = stderrWrites.find((entry) => entry.includes('dropped 20 log entries'));
  assert.ok(stderrSummary !== undefined, 'drop summary should also be printed to stderr');
}

export async function runJournaldSinkUnitTests(): Promise<void> {
  const previousStream = process.env.JOURNAL_STREAM;
  process.env.JOURNAL_STREAM = '1';
  const restore = installStubs();
  try {
    await testBuffersAndFlushes();
    await testDropSummary();
  } finally {
    restore();
    if (previousStream === undefined) {
      delete process.env.JOURNAL_STREAM;
    } else {
      process.env.JOURNAL_STREAM = previousStream;
    }
  }
}
