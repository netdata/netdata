import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import fs from 'node:fs';

import type { JournaldEmitter } from '../../logging/journald-sink.js';
import type { StructuredLogEvent } from '../../logging/structured-log-event.js';
import type { ChildProcess } from 'node:child_process';

interface JournaldTestModule {
  acquireJournaldSink: () => JournaldEmitter | undefined;
  __test: {
    resetSharedSink: () => void;
    setOverrides: (overrides: Partial<{
      spawn: (...args: unknown[]) => ChildProcess;
      statSync: typeof fs.statSync;
      accessSync: typeof fs.accessSync;
    }>) => void;
  };
}

class FakeWritable extends EventEmitter {
  public readonly writes: string[] = [];
  private callIndex = 0;
  private readonly backpressureCalls: Set<number>;

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

function createSink(module: JournaldTestModule, writer: FakeWritable): JournaldEmitter {
  module.__test.resetSharedSink();
  const realStatSync = (...args: Parameters<typeof fs.statSync>): ReturnType<typeof fs.statSync> => fs.statSync(...args);

  const overrideStatSync: typeof fs.statSync = ((...args: Parameters<typeof fs.statSync>) => {
    const [path] = args;
    if (typeof path === 'string') {
      if (path === '/run/systemd/journal/socket') {
        return {
          isSocket: () => true,
          isFile: () => false,
        } as unknown as fs.Stats;
      }
      if (path.includes('systemd-cat-native')) {
        return {
          isSocket: () => false,
          isFile: () => true,
        } as unknown as fs.Stats;
      }
    }
    return realStatSync(...args);
  }) as typeof fs.statSync;

  const overrideAccessSync: typeof fs.accessSync = ((..._args: Parameters<typeof fs.accessSync>) => undefined) as typeof fs.accessSync;
  const overrideSpawn = (..._args: unknown[]): ChildProcess => new FakeChildProcess(writer) as unknown as ChildProcess;

  module.__test.setOverrides({
    statSync: overrideStatSync,
    accessSync: overrideAccessSync,
    spawn: overrideSpawn,
  });

  const sink = module.acquireJournaldSink();
  assert.ok(sink !== undefined, 'journald sink should be acquired');
  return sink;
}

function testBuffersAndFlushes(module: JournaldTestModule): void {
  const writer = new FakeWritable([1]);
  const sink = createSink(module, writer);
  try {
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
  } finally {
    module.__test.resetSharedSink();
  }
}

function testDropSummary(module: JournaldTestModule): void {
  const writer = new FakeWritable([1]);
  const sink = createSink(module, writer);
  try {
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
  } finally {
    module.__test.resetSharedSink();
  }
}

export async function runJournaldSinkUnitTests(): Promise<void> {
  const previousStream = process.env.JOURNAL_STREAM;
  process.env.JOURNAL_STREAM = '1';
  const journaldModule = await import('../../logging/journald-sink.js') as unknown as JournaldTestModule;
  try {
    testBuffersAndFlushes(journaldModule);
    testDropSummary(journaldModule);
  } finally {
    journaldModule.__test.resetSharedSink();
    if (previousStream === undefined) {
      delete process.env.JOURNAL_STREAM;
    } else {
      process.env.JOURNAL_STREAM = previousStream;
    }
  }
}
