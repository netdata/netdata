"""Subprocess execution and bounded log capture.

Transport-free: this module knows nothing about MCP.  It provides a bounded,
offset-addressable log buffer and an async runner that streams a process'
merged stdout/stderr into such a buffer while staying cancellable.
"""

from __future__ import annotations

import asyncio
import os
import signal
from collections import deque
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from typing import Literal, Protocol

# Server-side long-poll cadence shared by the job and run status tools: hold a
# status call up to ~LONG_POLL_TIMEOUT, checking every LONG_POLL_INTERVAL.
LONG_POLL_TIMEOUT = 8.0
LONG_POLL_INTERVAL = 0.25


@dataclass
class LogSlice:
    """A view of buffered lines starting at a caller-held offset."""

    text: str
    next_offset: int
    truncated: bool  # True if lines before `offset` were already evicted


class LogBuffer:
    """A line-oriented ring buffer addressable by a monotonically growing offset.

    The buffer keeps only the most recent ``max_lines`` lines; older lines are
    evicted.  ``offset`` is the number of lines a reader has already consumed
    over the *whole* stream (not the current window), so a reader can keep
    reading incrementally even across evictions — it just learns, via
    ``truncated``, that some intermediate lines were dropped.
    """

    def __init__(self, max_lines: int = 10_000) -> None:
        self._lines: deque[str] = deque(maxlen=max_lines)
        self._dropped = 0  # count of lines evicted from the front

    def append(self, line: str) -> None:
        if len(self._lines) == self._lines.maxlen:
            # appending to a full deque evicts the leftmost line
            self._dropped += 1
        self._lines.append(line)

    @property
    def total(self) -> int:
        """Total lines ever appended (including evicted ones)."""
        return self._dropped + len(self._lines)

    def read(self, offset: int) -> LogSlice:
        total = self.total
        if offset < 0:
            offset = 0
        truncated = offset < self._dropped
        start = max(offset, self._dropped)
        window_idx = start - self._dropped
        new_lines = list(self._lines)[window_idx:]
        return LogSlice(text="\n".join(new_lines), next_offset=total, truncated=truncated)

    def tail(self, n: int) -> str:
        if n <= 0:
            return ""
        lines = list(self._lines)[-n:]
        return "\n".join(lines)


def kill_process_group(proc: asyncio.subprocess.Process | None, sig: int) -> None:
    """Best-effort signal the whole process group led by ``proc``.

    ``run_command`` spawns with ``start_new_session=True`` so the child is its
    own session/group leader (``pgid == pid``).  Signalling the group — not just
    the direct child — reaps grandchildren (e.g. ninja's gcc/cc1) that would
    otherwise keep the stdout pipe open and stall the reader.
    """
    if proc is None or proc.returncode is not None:
        return
    try:
        os.killpg(proc.pid, sig)
    except (ProcessLookupError, PermissionError):
        pass


class Cancellable(Protocol):
    """A unit of work the cancel helpers below can drive to a terminal state.

    :class:`~netdata_mcp.jobs.Job`, :class:`~netdata_mcp.run.Run`, and
    :class:`~netdata_mcp.streams.Stream` satisfy this structurally: a backing
    ``_task``, a ``done`` flag, and a SIGTERM (``request_cancel``) / SIGKILL
    (``force_kill``) pair on their process group.
    """

    _task: asyncio.Task | None

    @property
    def done(self) -> bool: ...
    def request_cancel(self) -> None: ...  # flag + SIGTERM the process group
    def force_kill(self) -> None: ...      # SIGKILL escalation


async def await_task(task: asyncio.Task | None, timeout: float) -> None:
    """Wait up to ``timeout`` for ``task`` to finish, swallowing the timeout.

    Shielded so a timeout here never cancels the task itself — the task owns its
    own finalization; we only observe it.
    """
    if task is None:
        return
    try:
        await asyncio.wait_for(asyncio.shield(task), timeout=timeout)
    except (asyncio.TimeoutError, asyncio.CancelledError):
        pass


async def escalate_cancel(handle: Cancellable, *, wait: float) -> None:
    """Cancel one unit of work: SIGTERM, grace, then SIGKILL if it ignored it.

    No-op if already done. The grace is capped at 3s so a single stop stays
    responsive even when the caller passes a long ``wait``.
    """
    if handle.done:
        return
    handle.request_cancel()
    await await_task(handle._task, min(wait, 3.0))
    if not handle.done:  # ignored SIGTERM -> escalate
        handle.force_kill()
        await await_task(handle._task, wait)


async def _drain(tasks: Iterable[asyncio.Task | None], timeout: float) -> None:
    live = [t for t in tasks if t is not None]
    if live:
        # asyncio.wait (unlike wait_for+gather) does NOT cancel pending tasks on
        # timeout. That matters: cancelling them would trip each unit's finally
        # safety-net to a terminal state and make the SIGKILL escalation below
        # see zero survivors.
        await asyncio.wait(live, timeout=timeout)


async def drain_all(handles: Iterable[Cancellable], *, wait: float) -> None:
    """Best-effort SIGTERM->SIGKILL of every live handle (used on shutdown)."""
    live = [h for h in handles if not h.done]
    if not live:
        return
    for h in live:
        h.request_cancel()  # SIGTERM the group
    await _drain([h._task for h in live], min(wait, 3.0))
    survivors = [h for h in live if not h.done]
    for h in survivors:
        h.force_kill()  # SIGKILL the group
    await _drain([h._task for h in survivors], wait)


async def run_command(
    cmd: list[str],
    cwd: str,
    sink: Callable[[str], None],
    on_spawn: Callable[[asyncio.subprocess.Process], None] | None = None,
    env: dict[str, str] | None = None,
) -> int:
    """Run ``cmd`` in ``cwd``, calling ``sink`` once per merged stdout/stderr line.

    Routing every line through a single ``sink`` lets the caller fan it out (log
    buffer, on-disk file, ...) and keep them identical.  ``on_spawn(proc)`` is
    invoked with the live process handle right after spawn so the caller can hold
    it for cancellation.  Returns the process exit code (negative when terminated
    by a signal).

    ``env`` (when given) is merged over the parent environment for the child only.
    Pass secrets (e.g. claim credentials) here rather than via ``cmd`` so they
    never appear in the process's command line / ``ps`` output.

    The child runs in a new session so the whole process tree can be cancelled
    together; if the read loop exits abnormally (e.g. a line over asyncio's
    64 KiB limit, or task cancellation) the group is force-terminated so no
    descendant survives holding the pipe.
    """
    proc = await asyncio.create_subprocess_exec(
        *cmd,
        cwd=cwd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
        start_new_session=True,
        env=({**os.environ, **env} if env else None),
    )
    if on_spawn is not None:
        on_spawn(proc)

    assert proc.stdout is not None
    try:
        async for raw in proc.stdout:
            sink(raw.decode(errors="replace").rstrip("\n"))
        return await proc.wait()
    finally:
        # Reached with returncode unset only on an abnormal exit (exception /
        # cancellation); terminate the group and reap so we don't leave a zombie.
        if proc.returncode is None:
            kill_process_group(proc, signal.SIGTERM)
            try:
                await asyncio.wait_for(asyncio.shield(proc.wait()), timeout=2.0)
            except (asyncio.TimeoutError, asyncio.CancelledError, ProcessLookupError):
                pass  # caller (cancel/cancel_all) handles SIGKILL escalation


class PhaseHost(Protocol):
    """A unit of work that runs an ordered list of build phases.

    Both :class:`~netdata_mcp.jobs.Job` and :class:`~netdata_mcp.run.Run` expose
    these same attributes, so :func:`run_phases` can drive either one's build
    section. The caller still owns mapping the outcome to its own terminal state.
    """

    current_phase: str | None
    _cancelled: bool
    _proc: asyncio.subprocess.Process | None

    def _set_proc(self, proc: asyncio.subprocess.Process) -> None: ...


@dataclass
class PhaseOutcome:
    """How a :func:`run_phases` loop ended; the caller maps it to its own state."""

    status: Literal["succeeded", "cancelled", "failed"]
    phase: str | None = None       # the phase that failed / was current at cancel
    returncode: int | None = None  # exit code of the last command run (None if cancelled before any)


async def run_phases(
    host: PhaseHost,
    phases: Iterable[tuple[str, list[str], str]],
    sink: Callable[[str], None],
) -> PhaseOutcome:
    """Run ``(name, cmd, cwd)`` phases in order, honoring cancellation between/around each.

    Announces each phase and streams its merged output through ``sink``; holds the
    live process on ``host`` (via ``_set_proc``) so a cancel can signal it, and
    clears it after. Returns as soon as a phase is cancelled or fails; otherwise
    runs them all and returns ``"succeeded"``. State mutation stays with the caller.
    """
    for name, cmd, cwd in phases:
        if host._cancelled:
            return PhaseOutcome("cancelled", name)
        host.current_phase = name
        sink(f"[phase: {name}] {' '.join(cmd)}")
        rc = await run_command(cmd, cwd, sink, on_spawn=host._set_proc)
        host._proc = None
        if host._cancelled:
            return PhaseOutcome("cancelled", name, rc)
        if rc != 0:
            return PhaseOutcome("failed", name, rc)
    # Reachable only with an empty phase list: a non-empty list is caught by the
    # per-iteration checks above (no await between the last one and here). Keeps
    # the contract honest for the degenerate case rather than reporting success.
    if host._cancelled:
        return PhaseOutcome("cancelled", None)
    return PhaseOutcome("succeeded")
