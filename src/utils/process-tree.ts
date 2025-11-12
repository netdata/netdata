import { spawnSync } from 'node:child_process';
import { readFile } from 'node:fs/promises';

type KillSignal = NodeJS.Signals | number;

export interface KillProcessTreeOptions {
  gracefulMs?: number;
  logger?: (message: string) => void;
  reason?: string;
  termSignal?: KillSignal;
  killSignal?: KillSignal;
}

export interface KillProcessTreeResult {
  attempted: number[];
  terminated: number[];
  stillRunning: number[];
  errors: { pid: number; error: string }[];
}

const DEFAULT_GRACE_MS = 2000;
const POSIX_PLATFORMS = new Set(['linux', 'darwin', 'freebsd']);

export async function killProcessTree(pid: number, opts?: KillProcessTreeOptions): Promise<KillProcessTreeResult> {
  if (!Number.isFinite(pid) || pid <= 0) {
    return { attempted: [], terminated: [], stillRunning: [], errors: [{ pid, error: 'invalid_pid' }] };
  }
  if (process.platform === 'win32') {
    return killOnWindows(pid, opts);
  }
  if (POSIX_PLATFORMS.has(process.platform)) {
    return killOnPosix(pid, opts);
  }
  // Fallback: just kill the single PID
  return killSingle(pid, opts);
}

async function killOnPosix(pid: number, opts?: KillProcessTreeOptions): Promise<KillProcessTreeResult> {
  const termSignal = opts?.termSignal ?? 'SIGTERM';
  const killSignal = opts?.killSignal ?? 'SIGKILL';
  const gracefulMs = opts?.gracefulMs ?? DEFAULT_GRACE_MS;
  const logger = opts?.logger;

  const descendants = await collectDescendants(pid);
  const ordered = [...descendants, pid]; // children first, then root

  const attempted: number[] = [];
  const terminated: number[] = [];
  const errors: { pid: number; error: string }[] = [];
  const pending = new Set<number>();

  ordered.forEach((target) => {
    attempted.push(target);
    if (sendSignal(target, termSignal, logger)) {
      pending.add(target);
    }
  });

  await waitForExit([...pending], gracefulMs);

  [...pending].forEach((target) => {
    if (!isAlive(target)) {
      terminated.push(target);
      pending.delete(target);
    }
  });

  [...pending].forEach((target) => {
    if (sendSignal(target, killSignal, logger)) {
      terminated.push(target);
      pending.delete(target);
    } else {
      errors.push({ pid: target, error: 'kill_failed' });
    }
  });

  const stillRunning = [...pending].filter((target) => isAlive(target));
  return { attempted, terminated, stillRunning, errors };
}

async function collectDescendants(rootPid: number): Promise<number[]> {
  const seen = new Set<number>();
  const queue: number[] = [rootPid];
  const result: number[] = [];
  let parentMap: Map<number, number[]> | undefined;
  // eslint-disable-next-line functional/no-loop-statements -- breadth-first traversal is clearer with a loop.
  while (queue.length > 0) {
    const current = queue.shift();
    if (current === undefined) break;
    let children: number[] = [];
    if (process.platform === 'linux') {
      const fromProc = await readProcChildren(current);
      if (fromProc !== undefined) {
        children = fromProc;
      }
    }
    if (children.length === 0) {
      const map = parentMap ?? buildParentMapViaPs();
      parentMap = map;
      children = getChildrenFromMap(current, map);
    }
    children.forEach((child) => {
      if (seen.has(child)) return;
      seen.add(child);
      result.push(child);
      queue.push(child);
    });
  }
  return result;
}

async function readProcChildren(pid: number): Promise<number[] | undefined> {
  try {
    const pidString = pid.toString();
    const raw = await readFile(`/proc/${pidString}/task/${pidString}/children`, 'utf8');
    return raw.split(' ').map((token) => Number.parseInt(token, 10)).filter((n) => Number.isFinite(n) && n > 0);
  } catch {
    return undefined;
  }
}

function buildParentMapViaPs(): Map<number, number[]> {
  const result = spawnSync('ps', ['-o', 'pid=', '-o', 'ppid=', '-ax'], { encoding: 'utf8' });
  const map = new Map<number, number[]>();
  if (result.error !== undefined || typeof result.stdout !== 'string') {
    return map;
  }
  const matcher = /^(\d+)\s+(\d+)$/;
  result.stdout.split('\n').forEach((line) => {
    const trimmed = line.trim();
    if (trimmed.length === 0) return;
    const match = matcher.exec(trimmed);
    if (match === null) return;
    const childPid = Number.parseInt(match[1], 10);
    const parentPid = Number.parseInt(match[2], 10);
    if (!Number.isFinite(childPid) || !Number.isFinite(parentPid)) return;
    const arr = map.get(parentPid) ?? [];
    arr.push(childPid);
    map.set(parentPid, arr);
  });
  return map;
}

function getChildrenFromMap(pid: number, map: Map<number, number[]>): number[] {
  const arr = map.get(pid);
  return Array.isArray(arr) ? arr : [];
}

async function killOnWindows(pid: number, opts?: KillProcessTreeOptions): Promise<KillProcessTreeResult> {
  const attempted = [pid];
  const terminated: number[] = [];
  const errors: { pid: number; error: string }[] = [];
  const gracefulMs = opts?.gracefulMs ?? DEFAULT_GRACE_MS;

  spawnSync('taskkill', ['/PID', String(pid), '/T'], { encoding: 'utf8' });
  await waitForExit([pid], gracefulMs);
  if (!isAlive(pid)) {
    terminated.push(pid);
    return { attempted, terminated, stillRunning: [], errors };
  }
  const forced = spawnSync('taskkill', ['/PID', String(pid), '/T', '/F'], { encoding: 'utf8' });
  if (forced.error !== undefined) {
    errors.push({ pid, error: forced.error.message });
  }
  if (!isAlive(pid)) {
    terminated.push(pid);
    return { attempted, terminated, stillRunning: [], errors };
  }
  return { attempted, terminated, stillRunning: [pid], errors };
}

async function killSingle(pid: number, opts?: KillProcessTreeOptions): Promise<KillProcessTreeResult> {
  const attempted = [pid];
  const termSignal = opts?.termSignal ?? 'SIGTERM';
  const killSignal = opts?.killSignal ?? 'SIGKILL';
  const gracefulMs = opts?.gracefulMs ?? DEFAULT_GRACE_MS;
  const errors: { pid: number; error: string }[] = [];

  if (!sendSignal(pid, termSignal, opts?.logger)) {
    return { attempted, terminated: [], stillRunning: [], errors: [{ pid, error: 'term_failed' }] };
  }
  await waitForExit([pid], gracefulMs);
  if (!isAlive(pid)) {
    return { attempted, terminated: [pid], stillRunning: [], errors };
  }
  if (!sendSignal(pid, killSignal, opts?.logger)) {
    errors.push({ pid, error: 'kill_failed' });
  }
  return { attempted, terminated: isAlive(pid) ? [] : [pid], stillRunning: isAlive(pid) ? [pid] : [], errors };
}

function sendSignal(pid: number, signal: KillSignal, logger?: (message: string) => void): boolean {
  if (!Number.isFinite(pid) || pid <= 0) return false;
  try {
    process.kill(pid, signal);
    return true;
  } catch (err) {
    const code = err instanceof Error && 'code' in err ? (err as { code?: string }).code : undefined;
    if (code === 'ESRCH') return false;
    const pidLabel = pid.toString();
    logger?.(`killProcessTree: failed to send ${String(signal)} to pid=${pidLabel}: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

function isAlive(pid: number): boolean {
  if (!Number.isFinite(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    const code = err instanceof Error && 'code' in err ? (err as { code?: string }).code : undefined;
    return code !== 'ESRCH';
  }
}

function waitForExit(pids: number[], timeoutMs: number): Promise<void> {
  if (pids.length === 0 || timeoutMs <= 0) return Promise.resolve();
  const intervalMs = 100;
  const maxChecks = Math.ceil(timeoutMs / intervalMs);
  let checks = 0;
  return new Promise((resolve) => {
    const timer = setInterval(() => {
      checks += 1;
      const anyAlive = pids.some((pid) => isAlive(pid));
      if (!anyAlive || checks >= maxChecks) {
        clearInterval(timer);
        resolve();
      }
    }, intervalMs);
  });
}
