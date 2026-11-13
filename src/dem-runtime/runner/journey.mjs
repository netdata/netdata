// SPDX-License-Identifier: GPL-3.0-or-later
import protocol from './protocol.cjs';

// The pinned Linux Node runtime uses CLOCK_MONOTONIC for hrtime. This origin is
// shared with the Go deadline handoff, not a portable Node API guarantee.
export const monotonicMilliseconds = () => Number(process.hrtime.bigint() / 1_000_000n);
export const budgetMessage = 'Journey execution budget exhausted before Playwright completed';
const maxTimerMilliseconds = 2_147_483_647;

export function journeyStop(child, stopAt, hasTerminal, clock = {
  now: monotonicMilliseconds, setTimeout, clearTimeout,
}) {
  let timer, exited = false, requested = false;
  const clear = () => { clock.clearTimeout(timer); timer = undefined; };
  child.once('exit', () => { exited = true; clear(); });
  child.once('error', clear);
  function arm() {
    if (exited || hasTerminal()) return;
    const remaining = stopAt - clock.now();
    if (remaining > 0) {
      // Node otherwise truncates an overflowing delay to one millisecond.
      timer = clock.setTimeout(arm, Math.min(remaining, maxTimerMilliseconds));
    } else {
      // Only this exact, still-owned CLI child receives the graceful request.
      requested = child.kill('SIGINT');
    }
  }
  arm();
  return {get requested() { return requested; }, clear};
}

export function journeyResult(terminal, budgetRequested, capture) {
  const result = {...terminal};
  if (budgetRequested) {
    result.status = 'timeout';
    result.termination_reason = 'execution_budget';
    if (!result.error) result.error = budgetMessage;
  } else {
    delete result.interrupted_artifacts;
  }
  result.capture_state = protocol.captureState(capture, result.status,
    [...(result.artifacts || []), ...(result.interrupted_artifacts || [])]);
  return result;
}
