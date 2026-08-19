"""Agent run lifecycle (transport-free: no MCP imports).

A run is per ``agent-id``: ensure the worktree's single ``build/`` is
built+installed for the profile (serialized per build dir, since that step is
shared), then launch netdata as a long-lived process and probe readiness. Stop
terminates the process group.

Distinct from a finite build job (jobs.py): the launch phase never completes on
its own, and readiness is derived by probing the agent's HTTP port.
"""

from __future__ import annotations

import asyncio
import signal
import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal

from . import buildcfg, runtime
from .locks import BuildLockCancelled, build_dir_lock
from .runner import (
    LONG_POLL_INTERVAL,
    LONG_POLL_TIMEOUT,
    LogBuffer,
    drain_all,
    escalate_cancel,
    kill_process_group,
    run_command,
    run_phases,
)

RunState = Literal["building", "starting", "ready", "stopped", "failed"]
_TERMINAL = frozenset({"stopped", "failed"})

# seconds for netdata to answer /api/v1/info after launch before we give up
_READINESS_TIMEOUT = 120.0
# grace after a timeout's SIGTERM before escalating to SIGKILL
_TIMEOUT_KILL_GRACE = 5.0

# injectable probe type: async (port) -> bool
Probe = Callable[[int], Awaitable[bool]]


@dataclass
class Run:
    agent_id: str
    worktree: str
    profile: str
    port: int
    run_dir: Path
    conf_path: Path
    otlp_endpoint: str = ""  # where the otel plugin listens for OTLP/gRPC data
    buffer: LogBuffer = field(default_factory=LogBuffer)
    state: RunState = "building"
    error: str | None = None
    returncode: int | None = None  # netdata's exit code once the launch ends
    claimed: bool | None = None  # has a claimed_id (refreshed when ready)
    cloud_connected: bool | None = None  # ACLK online / live in the Cloud UI
    current_phase: str | None = None
    created_at: float = field(default_factory=time.monotonic)

    _proc: asyncio.subprocess.Process | None = field(default=None, repr=False)
    _task: asyncio.Task | None = field(default=None, repr=False)
    _cancelled: bool = field(default=False, repr=False)

    @property
    def done(self) -> bool:
        return self.state in _TERMINAL

    @property
    def pid(self) -> int | None:
        """netdata's PID while the launch is alive, else None.

        Used to scope journal queries to this agent's worker processes (the
        otel-plugin workers are descendants of this PID).
        """
        p = self._proc
        return p.pid if (p is not None and p.returncode is None) else None

    def elapsed(self) -> float:
        return time.monotonic() - self.created_at

    def _set_proc(self, proc: asyncio.subprocess.Process) -> None:
        self._proc = proc

    def request_cancel(self) -> None:
        self._cancelled = True
        kill_process_group(self._proc, signal.SIGTERM)

    def force_kill(self) -> None:
        kill_process_group(self._proc, signal.SIGKILL)


class RunRegistry:
    def __init__(self) -> None:
        self._runs: dict[str, Run] = {}
        self._start_locks: dict[str, asyncio.Lock] = {}

    def get(self, agent_id: str) -> Run | None:
        return self._runs.get(agent_id)

    async def wait_status(
        self, agent_id: str, *, timeout: float = LONG_POLL_TIMEOUT, poll: float = LONG_POLL_INTERVAL
    ) -> Run | None:
        """Long-poll: return once the run leaves a coming-up state or timeout elapses."""
        run = self._runs.get(agent_id)
        if run is None:
            return None
        deadline = time.monotonic() + timeout
        while run.state in ("building", "starting") and time.monotonic() < deadline:
            await asyncio.sleep(poll)
        if run.state == "ready":
            # best-effort, never waited on: refresh claim/connection state. Update a
            # field only when the fetch yielded a known value — a transient HTTP
            # failure returns (None, None) and must not clobber a prior observation
            # (e.g. reset an already-seen cloud_connected=True back to None).
            claimed, connected = await runtime.cloud_status(run.port)
            if claimed is not None:
                run.claimed = claimed
            if connected is not None:
                run.cloud_connected = connected
        return run

    def _start_lock(self, agent_id: str) -> asyncio.Lock:
        # serialize start() per agent so concurrent calls can't both decide to
        # launch and overwrite _runs[agent_id], orphaning the loser's task.
        return self._start_locks.setdefault(agent_id, asyncio.Lock())

    async def start(
        self, agent_id: str, worktree: str, profile: str,
        *, otel: runtime.OtelConfig | None = None,
        restart: bool = False, probe: Probe | None = None,
    ) -> tuple[Run, str]:
        """Launch a run for an agent (fire-and-poll); return ``(run, outcome)``.

        ``outcome`` reports what actually happened so the caller can message
        honestly instead of always claiming a start:

        - ``"already-running"`` - a live run existed and ``restart`` was False;
          the live run is returned unchanged (idempotent, source edits NOT
          picked up).
        - ``"restarted"`` - a live run existed and ``restart`` was True; it was
          stopped and a fresh run (rebuild via ``ninja install`` + relaunch)
          started.
        - ``"started"`` - no live run existed; a fresh run started.
        """
        # Held across the decide + stop + create so a concurrent start for the
        # same agent observes the fresh run, not the one we're about to replace.
        async with self._start_lock(agent_id):
            existing = self._runs.get(agent_id)
            if existing is not None and not existing.done:
                if not restart:
                    return existing, "already-running"
                await self.stop(agent_id)  # tear the live run down before relaunching
                outcome = "restarted"
            else:
                outcome = "started"

            port = runtime.free_port()
            rd, conf, otlp_endpoint = runtime.generate_runtime(agent_id, otel=otel)
            run = Run(
                agent_id=agent_id, worktree=worktree, profile=profile,
                port=port, run_dir=rd, conf_path=conf, otlp_endpoint=otlp_endpoint,
            )
            self._runs[agent_id] = run
            run._task = asyncio.get_running_loop().create_task(
                self._drive(run, probe or runtime.probe_ready)
            )
        return run, outcome

    async def _drive(self, run: Run, probe: Probe) -> None:
        try:
            if await self._build_and_install(run):
                await self._launch_and_settle(run, probe)
        except BuildLockCancelled:  # stopped while queued for the build-dir lock
            run.state = "stopped"
            run.buffer.append("[stopped while waiting for the build-dir lock]")
        except Exception as exc:  # surface, never crash the server
            run.error = str(exc)
            run.state = "stopped" if run._cancelled else "failed"
            run.buffer.append(f"[error: {exc}]")
        finally:
            # task cancelled externally (CancelledError bypasses except) ->
            # never leave a non-terminal state
            if run.state not in _TERMINAL:
                run.state = "stopped"

    async def _build_and_install(self, run: Run) -> bool:
        """Configure (if needed) + install the worktree under the build-dir lock.

        Returns True to proceed to launch; False if it ended terminally (cancelled
        or failed — the run's state is already set). The build dir is shared, so
        this step is serialized per dir by the lock.
        """
        # Claim the build dir before configuring (refuse a foreign one); doing it
        # before configure keeps a failed/killed configure recoverable.
        buildcfg.claim_build_dir(run.worktree)
        wt = run.worktree
        phases: list[tuple[str, list[str], str]] = []
        if buildcfg.needs_configure(wt, run.profile):
            phases.append(("configure", buildcfg.configure_command(wt, run.profile), wt))
        phases.append(("install", buildcfg.install_command(wt), wt))

        async with build_dir_lock(
            buildcfg.lock_file(wt),
            cancel_check=lambda: run._cancelled,
        ):
            outcome = await run_phases(run, phases, run.buffer.append)
        if outcome.status == "cancelled":
            run.state = "stopped"
            return False
        if outcome.status == "failed":
            run.error = f"{outcome.phase} failed (exit {outcome.returncode})"
            run.state = "failed"
            run.buffer.append(f"[{outcome.phase} failed with exit {outcome.returncode}]")
            return False
        return True

    async def _launch_and_settle(self, run: Run, probe: Probe) -> None:
        """Launch netdata (long-lived), probe readiness, then finalize the run.

        On readiness timeout, escalates SIGTERM -> SIGKILL so an agent ignoring
        SIGTERM can't block forever; records netdata's exit code in either case.
        """
        if run._cancelled:
            run.state = "stopped"
            return
        run.current_phase = "launch"
        run.state = "starting"
        netdata = runtime.install_bin(run.worktree)
        cmd = runtime.launch_command(netdata, run.port, run.conf_path)
        # Claim creds (if configured) go in the launch env, never the command line.
        # No creds -> launch unclaimed; claiming is best-effort and never blocks
        # readiness (the daemon claims at startup; cloud connection is async).
        claim = runtime.claim_env()
        if claim:
            run.buffer.append(f"[claim] claiming to Cloud as mcp-{run.agent_id} (ephemeral)")
        else:
            run.buffer.append("[claim] NETDATA_CLAIM_TOKEN not set — running unclaimed")
        # When not claiming, blank the token in the child env: a stray/whitespace
        # NETDATA_CLAIM_TOKEN inherited from the server env would otherwise reach
        # netdata (which treats any non-empty token as a claim request) and trigger
        # a doomed claim attempt with its ~50s startup tail.
        launch_env = claim or {"NETDATA_CLAIM_TOKEN": ""}
        run.buffer.append(f"[phase: launch] {' '.join(cmd)}")
        launch = asyncio.get_running_loop().create_task(
            run_command(cmd, run.worktree, run.buffer.append, on_spawn=run._set_proc, env=launch_env)
        )
        deadline = time.monotonic() + _READINESS_TIMEOUT
        timed_out = False
        while not launch.done():
            if run._cancelled:
                break
            if await probe(run.port):
                run.state = "ready"
                break
            if time.monotonic() > deadline:
                timed_out = True
                run.buffer.append(f"[readiness timeout after {_READINESS_TIMEOUT:.0f}s]")
                kill_process_group(run._proc, signal.SIGTERM)
                break
            await asyncio.sleep(0.5)
        # Finalize. A user cancel wins over a racing timeout. A timeout that is NOT
        # a cancel escalates SIGTERM -> SIGKILL so an agent ignoring SIGTERM can't
        # block forever; state is set before awaiting so a late CancelledError
        # can't demote it.
        if timed_out and not run._cancelled:
            run.error = f"agent did not become ready within {_READINESS_TIMEOUT:.0f}s"
            run.state = "failed"
            try:
                await asyncio.wait_for(asyncio.shield(launch), timeout=_TIMEOUT_KILL_GRACE)
            except asyncio.TimeoutError:
                run.buffer.append("[SIGKILL escalation: agent ignored SIGTERM]")
                run.force_kill()
                await launch
            except Exception as exc:  # keep the timeout diagnostic in run.error
                run.buffer.append(f"[grace-period error: {exc}]")
            if run._cancelled:  # a stop racing the escalation still wins
                run.state = "stopped"
        else:
            # ready: park for the agent's lifetime; cancel: stop() drives the
            # SIGTERM->SIGKILL escalation and we rejoin when netdata exits.
            await launch
            run.state = "stopped" if run._cancelled else "failed"
        # launch is done on every path above; record netdata's exit code (negative
        # = killed by signal). result() re-raises if run_command raised — the error
        # is already handled, so ignore it here.
        try:
            run.returncode = launch.result()
        except Exception:
            pass
        run._proc = None

    async def stop(self, agent_id: str, *, wait: float = 5.0) -> Run | None:
        run = self._runs.get(agent_id)
        if run is None:
            return None
        await escalate_cancel(run, wait=wait)
        return run

    async def stop_all(self, *, wait: float = 10.0) -> None:
        await drain_all(self._runs.values(), wait=wait)
