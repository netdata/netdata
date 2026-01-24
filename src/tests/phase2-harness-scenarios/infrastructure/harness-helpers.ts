/**
 * Utility functions for the Phase 2 deterministic test harness.
 * Extracted from phase2-runner.ts for modularity.
 */

import type { ConversationMessage, LogDetailValue, LogEntry } from '../../../types.js';

import { LOG_EVENTS } from '../../../logging/log-events.js';

/**
 * Assert that a condition is true, throwing an error with the given message if not.
 */
export function invariant(condition: boolean, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

/**
 * Convert an unknown value to an Error instance.
 */
export const toError = (value: unknown): Error => (value instanceof Error ? value : new Error(String(value)));

/**
 * Extract an error message from an unknown value.
 */
export const toErrorMessage = (value: unknown): string => (value instanceof Error ? value.message : String(value));

/**
 * Type guard to check if a value is a plain object (Record<string, unknown>).
 */
export const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

/**
 * Promise-based delay function.
 */
export const delay = (ms: number): Promise<void> => new Promise((resolve) => { setTimeout(resolve, ms); });

/**
 * Create a deferred promise with external resolve/reject functions.
 */
export function createDeferred<T = void>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
} {
  let resolveFn: ((value: T) => void) | undefined;
  let rejectFn: ((reason?: unknown) => void) | undefined;
  const promise = new Promise<T>((resolve, reject) => {
    resolveFn = resolve;
    rejectFn = reject;
  });
  const resolve = (value: T): void => { resolveFn?.(value); };
  const reject = (reason?: unknown): void => { rejectFn?.(reason); };
  return { promise, resolve, reject };
}

/** Decimal precision for duration formatting */
export const RUN_LOG_DECIMAL_PRECISION = 2;

/**
 * Format a duration in milliseconds as a human-readable string.
 */
export function formatDurationMs(startMs: number, endMs: number): string {
  const seconds = (endMs - startMs) / 1000;
  return `${seconds.toFixed(RUN_LOG_DECIMAL_PRECISION)}s`;
}

/** Maximum recursion depth to prevent stack overflow */
const SAFE_JSON_MAX_DEPTH = 32;

/**
 * Check if a value is JSON-serializable (not undefined, function, or symbol).
 * JSON.stringify skips object properties with these values.
 */
const isJsonSerializable = (value: unknown): boolean =>
  value !== undefined && typeof value !== 'function' && typeof value !== 'symbol';

/**
 * Safely estimate the byte length of a JSON-serialized value.
 * Uses depth-limited manual traversal to prevent stack overflow on cyclic or deeply nested objects.
 * Follows JSON.stringify semantics: undefined/function/symbol in objects are skipped,
 * undefined/function/symbol in arrays become "null", boxed primitives are unboxed.
 * Note: Does not handle toJSON (Date, custom objects) or BigInt - returns approximate size.
 */
export const safeJsonByteLengthLocal = (value: unknown, depth = 0, inArray = false): number => {
  // Prevent stack overflow on deeply nested objects
  if (depth > SAFE_JSON_MAX_DEPTH) return 0;

  // Handle primitives directly (safe, no recursion risk)
  if (value === null) return 4; // "null"
  // JSON.stringify(undefined) at top level returns undefined (no output)
  // In arrays: undefined/function/symbol become "null"
  // In objects: properties with these values are skipped (handled at call site)
  if (value === undefined || typeof value === 'function' || typeof value === 'symbol') {
    return inArray ? 4 : 0; // "null" in arrays, skip otherwise
  }
  if (typeof value === 'string') return Buffer.byteLength(JSON.stringify(value), 'utf8');
  if (typeof value === 'number') {
    // JSON.stringify converts NaN/Infinity to "null"
    if (!Number.isFinite(value)) return 4; // "null"
    return String(value).length;
  }
  if (typeof value === 'boolean') return value ? 4 : 5; // "true" or "false"

  // Handle boxed primitives (new Number(), new String(), new Boolean())
  // JSON.stringify unboxes these to their primitive values
  if (value instanceof Number) {
    const num = value.valueOf();
    if (!Number.isFinite(num)) return 4; // "null"
    return String(num).length;
  }
  if (value instanceof String) {
    return Buffer.byteLength(JSON.stringify(value.valueOf()), 'utf8');
  }
  if (value instanceof Boolean) {
    return value.valueOf() ? 4 : 5; // "true" or "false"
  }

  // For objects/arrays, always use manual depth-limited traversal (guaranteed stack safe)
  if (typeof value === 'object') {
    const isArray = Array.isArray(value);

    if (isArray) {
      const arr = value as unknown[];
      if (arr.length === 0) return 2; // []
      let total = 2; // [ and ]
      // Use for loop to handle sparse arrays (forEach skips holes)
      // eslint-disable-next-line functional/no-loop-statements
      for (let i = 0; i < arr.length; i += 1) {
        if (i > 0) total += 1; // comma
        // Array holes become "null" in JSON
        if (!(i in arr)) {
          total += 4; // "null"
        } else {
          // Pass inArray=true so undefined/function/symbol return 4 ("null")
          total += safeJsonByteLengthLocal(arr[i], depth + 1, true);
        }
      }
      return total;
    }

    // Object handling - skip undefined/function/symbol values per JSON.stringify semantics
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj).filter((k) => isJsonSerializable(obj[k]));
    if (keys.length === 0) return 2; // {}

    let total = 2; // { and }
    keys.forEach((key, idx) => {
      if (idx > 0) total += 1; // comma
      total += Buffer.byteLength(JSON.stringify(key), 'utf8'); // key with quotes
      total += 1; // colon
      total += safeJsonByteLengthLocal(obj[key], depth + 1);
    });
    return total;
  }

  return 0;
};

/**
 * Estimate the byte size of a list of conversation messages.
 */
export const estimateMessagesBytesLocal = (messages: readonly ConversationMessage[]): number => {
  if (messages.length === 0) {
    return 0;
  }
  return messages.reduce((total, message) => total + safeJsonByteLengthLocal(message), 0);
};

/**
 * Check if a log entry has a specific detail key.
 */
export function logHasDetail(entry: LogEntry, key: string): boolean {
  const details = entry.details;
  return details !== undefined && Object.prototype.hasOwnProperty.call(details, key);
}

/**
 * Get a detail value from a log entry.
 */
export function getLogDetail(entry: LogEntry, key: string): LogDetailValue | undefined {
  const details = entry.details;
  if (details === undefined) {
    return undefined;
  }
  if (!Object.prototype.hasOwnProperty.call(details, key)) {
    return undefined;
  }
  return details[key];
}

/**
 * Assert that a log entry has a numeric detail value and return it.
 */
export function expectLogDetailNumber(entry: LogEntry, key: string, message: string): number {
  const value = getLogDetail(entry, key);
  invariant(typeof value === 'number' && Number.isFinite(value), message);
  return value;
}

/**
 * Check if a log entry has a specific event type.
 */
export const logHasEvent = (entry: LogEntry, event: LogDetailValue): boolean => getLogDetail(entry, 'event') === event;

/**
 * Get all log entries with a specific remote identifier.
 */
export const getLogsByIdentifier = (logs: readonly LogEntry[], identifier: string): LogEntry[] =>
  logs.filter((entry) => entry.remoteIdentifier === identifier);

/**
 * Find the first log entry with a specific remote identifier.
 */
export const findLogByIdentifier = (
  logs: readonly LogEntry[],
  identifier: string,
  predicate?: (entry: LogEntry) => boolean
): LogEntry | undefined => {
  if (predicate === undefined) return logs.find((entry) => entry.remoteIdentifier === identifier);
  return logs.find((entry) => entry.remoteIdentifier === identifier && predicate(entry));
};

/**
 * Find the first log entry with a specific event type.
 */
export const findLogByEvent = (
  logs: readonly LogEntry[],
  event: LogDetailValue,
  predicate?: (entry: LogEntry) => boolean
): LogEntry | undefined => {
  if (predicate === undefined) {
    return logs.find((entry) => logHasEvent(entry, event));
  }
  return logs.find((entry) => logHasEvent(entry, event) && predicate(entry));
};

/**
 * Assert that a log entry with a specific identifier and event exists.
 */
export const expectLogEvent = (
  logs: readonly LogEntry[],
  identifier: string,
  event: LogDetailValue,
  scenarioId: string
): void => {
  const log = findLogByIdentifier(logs, identifier, (entry) => logHasEvent(entry, event));
  invariant(log !== undefined, `Expected log ${identifier} with event "${String(event)}" for ${scenarioId}.`);
};

/** Remote identifier for turn failure logs */
export const LOG_TURN_FAILURE = 'agent:turn-failure';

/**
 * Assert that turn failure log exists with required slugs.
 */
export const expectTurnFailureSlugs = (logs: readonly LogEntry[], scenarioId: string, required: string[]): LogEntry => {
  // Use findLast to get the final turn failure log (retries_exhausted only appears in the last one)
  const entry = logs.findLast((log) => log.remoteIdentifier === LOG_TURN_FAILURE && log.severity === 'WRN');
  invariant(entry !== undefined, `Turn failure log missing for ${scenarioId}.`);
  const slugStr = typeof entry.details?.slugs === 'string' ? entry.details.slugs : '';
  const slugs = slugStr.split(',').filter((s) => s.length > 0);
  required.forEach((slug) => {
    invariant(slugs.includes(slug), `Expected slug '${slug}' for ${scenarioId}; got [${slugs.join(',')}].`);
  });
  return entry;
};

/**
 * Assert that turn failure log contains specific slugs.
 */
export const expectTurnFailureContains = (logs: readonly LogEntry[], scenarioId: string, required: string[]): void => {
  const entry = expectTurnFailureSlugs(logs, scenarioId, []);
  const slugStr = typeof entry.details?.slugs === 'string' ? entry.details.slugs : '';
  const slugs = slugStr.split(',').filter((s) => s.length > 0);
  required.forEach((slug) => {
    invariant(slugs.includes(slug), `Expected slug '${slug}' for ${scenarioId}; got [${slugs.join(',')}].`);
  });
};

/**
 * Extract the nonce from conversation messages for XML tag matching.
 */
export const extractNonceFromMessages = (messages: readonly ConversationMessage[], scenarioId: string): string => {
  const asString = (msg: ConversationMessage): string | undefined =>
    typeof msg.content === 'string' ? msg.content : undefined;

  // 1) Prefer explicit Nonce line in xml-next
  const notice = messages.find((msg) => msg.noticeType === 'xml-next')
    ?? messages.find((msg) => msg.role === 'user' && typeof msg.content === 'string' && msg.content.includes('Nonce:'));
  const noticeContent = notice !== undefined ? asString(notice) : undefined;
  if (noticeContent !== undefined) {
    const matchLine = /Nonce:\s*([a-z0-9]+)/i.exec(noticeContent);
    if (matchLine !== null && typeof matchLine[1] === 'string' && matchLine[1].length > 0) {
      return matchLine[1];
    }
    const matchSlotNotice = /<ai-agent-([a-z0-9]+)-(FINAL|PROGRESS|0001)/i.exec(noticeContent);
    if (matchSlotNotice !== null && typeof matchSlotNotice[1] === 'string' && matchSlotNotice[1].length > 0) {
      return matchSlotNotice[1];
    }
  }

  // 2) Fallback: scan all messages (system prompt included) for any ai-agent-* tag
  const tagNonce = messages
    .map(asString)
    .filter((c): c is string => c !== undefined)
    .map((content) => /<ai-agent-([a-z0-9]+)-(FINAL|PROGRESS|0001)/i.exec(content))
    .find((m) => m !== null && typeof m[1] === 'string' && m[1].length > 0);
  if (tagNonce !== undefined && tagNonce !== null) {
    return tagNonce[1];
  }

  invariant(false, `Nonce parse failed for ${scenarioId}.`);
};

/**
 * Find a TURN-FAILED message containing a specific fragment.
 */
export function findTurnFailedMessage(conversation: ConversationMessage[], fragment: string): ConversationMessage | undefined {
  return conversation.find((message) =>
    message.role === 'user'
    && typeof message.content === 'string'
    && message.content.includes('TURN-FAILED')
    && message.content.includes(fragment)
  );
}

/**
 * Strip ANSI escape codes from a string.
 */
export function stripAnsiCodes(value: string): string {
  return value.replace(/\u001B\[[0-9;]*m/g, '');
}

/**
 * Get a private method from an object instance via reflection.
 */
export function getPrivateMethod(instance: object, key: string): (...args: unknown[]) => unknown {
  const value = Reflect.get(instance as Record<string, unknown>, key);
  invariant(typeof value === 'function', `Private method '${key}' missing.`);
  return value as (...args: unknown[]) => unknown;
}

/**
 * Get a private field from an object instance via reflection.
 */
export function getPrivateField(instance: object, key: string): unknown {
  return Reflect.get(instance as Record<string, unknown>, key);
}

/**
 * Assert that a value is a plain object (Record<string, unknown>).
 */
export function assertRecord(value: unknown, message: string): asserts value is Record<string, unknown> {
  invariant(value !== null && typeof value === 'object' && !Array.isArray(value), message);
}

/**
 * Expect a value to be a plain object and return it typed.
 */
export function expectRecord(value: unknown, message: string): Record<string, unknown> {
  assertRecord(value, message);
  return value;
}

/**
 * Decode a base64 string to UTF-8.
 */
export function decodeBase64(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.length === 0) return undefined;
  try {
    return Buffer.from(value, 'base64').toString('utf8');
  } catch {
    return undefined;
  }
}

/**
 * Run a promise with a timeout.
 */
export function runWithTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  scenarioId: string,
  onTimeout?: () => void
): Promise<T> {
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) return promise;
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    let timer: ReturnType<typeof setTimeout>;
    const finalize = (action: () => void): void => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      action();
    };
    timer = setTimeout(() => {
      finalize(() => {
        try { onTimeout?.(); } catch { /* ignore */ }
        reject(new Error(`Scenario ${scenarioId} timed out after ${String(timeoutMs)} ms`));
      });
    }, timeoutMs);
    void (async () => {
      try {
        const value = await promise;
        finalize(() => { resolve(value); });
      } catch (error: unknown) {
        finalize(() => { reject(toError(error)); });
      }
    })();
  });
}

/**
 * Parse a comma-separated list into an array of trimmed, non-empty strings.
 */
export const parseDumpList = (raw?: string): string[] => {
  if (typeof raw !== 'string') return [];
  return raw.split(',').map((value) => value.trim()).filter((value) => value.length > 0);
};

/**
 * Parse a scenario filter from environment variable format.
 */
export const parseScenarioFilter = (raw?: string): string[] => {
  if (typeof raw !== 'string') return [];
  return raw.split(',').map((value) => value.trim()).filter((value) => value.length > 0);
};

// Re-export LOG_EVENTS for convenience
export { LOG_EVENTS };
