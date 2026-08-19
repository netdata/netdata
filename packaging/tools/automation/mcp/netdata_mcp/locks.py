"""Cross-process build-dir lock (transport-free: no MCP imports).

A worktree's build dir (``<worktree>/build``) is a filesystem resource that any
number of coroutines, both job registries, and — under stdio transport, the
normal case — separate server processes may try to write via cmake/ninja. An
in-process ``asyncio.Lock`` cannot span processes, so mutual exclusion lives on
the resource itself: an advisory file lock on a lockfile beside the build dir.
``filelock`` uses ``fcntl.flock`` on Unix, so the kernel releases the lock when
the holding process dies — a crashed server never leaves a stale lock. The
lockfile sits at the worktree root (a sibling of ``build/``, gitignored), so a
``rm -rf build/`` mid-build can't drop a lock another process is holding.
"""

from __future__ import annotations

from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from pathlib import Path

from filelock import AsyncFileLock, Timeout


class BuildLockCancelled(Exception):
    """A build-dir lock wait was aborted by ``cancel_check`` — the caller was
    cancelled/stopped while still queued for the lock, before acquiring it."""


@asynccontextmanager
async def build_dir_lock(
    lockfile: Path, cancel_check: Callable[[], bool] | None = None
) -> AsyncIterator[None]:
    """Hold an exclusive cross-process lock on ``lockfile`` for the block.

    A fresh ``AsyncFileLock`` per call: filelock is re-entrant per instance, so
    reusing one instance across coroutines would collapse mutual exclusion. The
    acquire polls with ``await`` between attempts, so it serialises (waits) and
    stays cancellable.

    ``cancel_check`` is polled between acquire attempts; when it returns True the
    wait is abandoned with :class:`BuildLockCancelled` instead of blocking until
    the lock frees. This lets a job/run that is *queued* on the lock stop
    promptly on cancel rather than waiting out the holder (which may build for
    minutes in another process).
    """
    lockfile.parent.mkdir(parents=True, exist_ok=True)
    lock = AsyncFileLock(str(lockfile))
    try:
        # No timeout (poll forever) — the only way acquire times out here is
        # cancel_check firing, which filelock surfaces as Timeout.
        await lock.acquire(poll_interval=0.1, cancel_check=cancel_check)
    except Timeout:
        if cancel_check is not None and cancel_check():
            raise BuildLockCancelled() from None
        raise
    try:
        yield
    finally:
        await lock.release()
