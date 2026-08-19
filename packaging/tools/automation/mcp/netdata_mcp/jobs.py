"""Generic background-job registry.

Transport-free: no MCP imports.  A :class:`Job` runs an ordered list of
:class:`Phase` commands as a single unit of work, streaming their merged output
into a :class:`~netdata_mcp.runner.LogBuffer`.  The :class:`JobRegistry` owns job
lifecycle, a per-resource lock that serialises jobs touching the same resource
(e.g. one build dir), and graceful handling of unknown job ids.

Jobs are in-memory only; they do not survive a server restart.
"""

from __future__ import annotations

import asyncio
import signal
import time
from contextlib import nullcontext
from dataclasses import dataclass, field
from pathlib import Path
from typing import IO, Literal

from .locks import BuildLockCancelled, build_dir_lock
from .runner import (
    LONG_POLL_INTERVAL,
    LONG_POLL_TIMEOUT,
    LogBuffer,
    LogSlice,
    drain_all,
    escalate_cancel,
    kill_process_group,
    run_phases,
)

JobState = Literal["running", "succeeded", "failed", "cancelled"]
StartOutcome = Literal["started", "deduped", "busy"]


@dataclass
class Phase:
    """One command in a job."""

    name: str
    cmd: list[str]
    cwd: str


@dataclass
class Job:
    id: str
    kind: str          # e.g. "configure" | "build"
    profile: str
    key: str           # shared resource key (serialised by the registry)
    worktree: str
    phases: list[Phase]
    log_path: Path | None = None  # full log teed here (unbounded), for post-mortem grep
    lockfile: Path | None = None  # cross-process build-dir lock held across phases
    buffer: LogBuffer = field(default_factory=LogBuffer)
    state: JobState = "running"
    returncode: int | None = None
    error: str | None = None
    current_phase: str | None = None
    created_at: float = field(default_factory=time.monotonic)
    finished_at: float | None = None

    _proc: asyncio.subprocess.Process | None = field(default=None, repr=False)
    _task: asyncio.Task | None = field(default=None, repr=False)
    _cancelled: bool = field(default=False, repr=False)

    @property
    def signature(self) -> tuple[str, str]:
        """Identifies an equivalent request for dedup (same kind + profile)."""
        return (self.kind, self.profile)

    @property
    def done(self) -> bool:
        return self.state != "running"

    def elapsed(self) -> float:
        end = self.finished_at if self.finished_at is not None else time.monotonic()
        return end - self.created_at

    def start(self) -> None:
        self._task = asyncio.get_running_loop().create_task(self._run())

    def _set_proc(self, proc: asyncio.subprocess.Process) -> None:
        self._proc = proc

    def _open_log(self) -> IO[str] | None:
        if self.log_path is None:
            return None
        try:
            self.log_path.parent.mkdir(parents=True, exist_ok=True)
            return open(self.log_path, "w", buffering=1, encoding="utf-8", errors="replace")
        except OSError as exc:
            self.buffer.append(f"[warn: could not open log file {self.log_path}: {exc}]")
            return None

    async def _run(self) -> None:
        # Opened inside the lock below (not here): the log file is a shared
        # build-dir resource, so a job queued on the lock must not truncate the
        # holder's in-progress log before it even acquires the lock.
        fh = None

        def emit(line: str) -> None:
            nonlocal fh
            self.buffer.append(line)
            if fh is not None:
                try:
                    fh.write(line + "\n")
                except OSError as exc:
                    # Stop after the first failure so we don't silently drop
                    # every subsequent line; the buffer still has them. Close the
                    # handle now (the finally below will skip it once fh is None).
                    self.buffer.append(f"[warn: log file write failed ({exc}); disk logging stopped]")
                    try:
                        fh.close()
                    except OSError:
                        pass
                    fh = None

        # Hold the cross-process build-dir lock across all phases when set, so a
        # build job and another builder (the other registry, or another server
        # process) never run cmake/ninja in one dir concurrently. Early returns
        # and exceptions both exit the `async with`, releasing the lock.
        lock_cm = (
            build_dir_lock(self.lockfile, cancel_check=lambda: self._cancelled)
            if self.lockfile is not None
            else nullcontext()
        )
        try:
            async with lock_cm:
                fh = self._open_log()  # only the lock holder writes the build log
                outcome = await run_phases(
                    self, [(p.name, p.cmd, p.cwd) for p in self.phases], emit
                )
                if outcome.status == "cancelled":
                    self.returncode = outcome.returncode
                    self.state = "cancelled"
                    return
                if outcome.status == "failed":
                    self.returncode = outcome.returncode
                    self.state = "failed"
                    emit(f"[phase '{outcome.phase}' failed with exit {outcome.returncode}]")
                    return
                self.returncode = 0
                self.state = "succeeded"
        except BuildLockCancelled:  # cancelled while queued for the build-dir lock
            self.state = "cancelled"
            emit("[cancelled while waiting for the build-dir lock]")
        except Exception as exc:  # surface, do not crash the server
            # An exception that races with a cancel request is still a cancel.
            if self._cancelled:
                self.state = "cancelled"
            else:
                self.error = str(exc)
                self.state = "failed"
            emit(f"[error: {exc}]")
        finally:
            self.finished_at = time.monotonic()
            # Safety net: if the task was cancelled externally (CancelledError is
            # a BaseException and bypasses the handler above), never leave the job
            # stuck in "running".
            if self.state == "running":
                self.state = "cancelled"
            if fh is not None:
                fh.close()

    def request_cancel(self) -> None:
        """Politely stop the job: flag it and SIGTERM the whole process group."""
        self._cancelled = True
        kill_process_group(self._proc, signal.SIGTERM)

    def force_kill(self) -> None:
        """Escalation: SIGKILL the process group when SIGTERM was ignored."""
        kill_process_group(self._proc, signal.SIGKILL)


@dataclass
class StartResult:
    job: Job
    outcome: StartOutcome


class JobRegistry:
    def __init__(self) -> None:
        self._jobs: dict[str, Job] = {}
        self._by_key: dict[str, Job] = {}
        self._lock = asyncio.Lock()
        self._counter = 0

    async def start(
        self,
        *,
        kind: str,
        profile: str,
        key: str,
        worktree: str,
        phases: list[Phase],
        log_path: Path | None = None,
        lockfile: Path | None = None,
    ) -> StartResult:
        async with self._lock:
            running = self._by_key.get(key)
            if running is not None and not running.done:
                if running.signature == (kind, profile):
                    return StartResult(running, "deduped")
                return StartResult(running, "busy")

            self._counter += 1
            job = Job(
                id=f"job-{self._counter}",
                kind=kind,
                profile=profile,
                key=key,
                worktree=worktree,
                phases=phases,
                log_path=log_path,
                lockfile=lockfile,
            )
            self._jobs[job.id] = job
            self._by_key[key] = job
            job.start()
            return StartResult(job, "started")

    def get(self, job_id: str) -> Job | None:
        return self._jobs.get(job_id)

    async def wait_status(self, job_id: str, *, timeout: float = LONG_POLL_TIMEOUT, poll: float = LONG_POLL_INTERVAL) -> Job | None:
        """Long-poll: return once the job leaves ``running`` or ``timeout`` elapses."""
        job = self.get(job_id)
        if job is None:
            return None
        deadline = time.monotonic() + timeout
        while not job.done and time.monotonic() < deadline:
            await asyncio.sleep(poll)
        return job

    def logs(self, job_id: str, offset: int) -> tuple[Job, LogSlice] | None:
        job = self.get(job_id)
        if job is None:
            return None
        return job, job.buffer.read(offset)

    async def cancel(self, job_id: str, *, wait: float = 5.0) -> Job | None:
        job = self.get(job_id)
        if job is None:
            return None
        await escalate_cancel(job, wait=wait)
        return job

    async def cancel_all(self, *, wait: float = 10.0) -> None:
        """Best-effort cancellation of all running jobs (used on shutdown)."""
        await drain_all(self._jobs.values(), wait=wait)
