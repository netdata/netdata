"""Run otel-streams generators against an agent's OTLP endpoint (transport-free).

Both shapes shell out to ``cargo run -p otel-streams --bin <name>`` from
``<worktree>/src/crates`` — otel-streams is NOT built by the agent's cmake build,
so cargo builds it on demand (first call) and caches it after, always matching
the worktree source.

- **synth** is one-shot: :func:`run_synth` runs it to completion and returns the
  outcome.
- **certstream / jetstream / github** are long-running daemons:
  :class:`StreamRegistry` start/status/stops them like ``RunRegistry`` does for
  agents, reusing ``runner.py``'s process-group spawn + SIGTERM→SIGKILL teardown.

No Cloud token / bearer is involved (the push targets the agent's local OTLP/gRPC
receiver, not the access-gated function endpoint), so there is no secret surface
here; ``tenant_id`` is an identifier, not a credential.
"""

from __future__ import annotations

import asyncio
import itertools
import signal
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal

from .runner import (
    LogBuffer,
    drain_all,
    escalate_cancel,
    kill_process_group,
    run_command,
)

StreamSource = Literal["certstream", "jetstream", "github"]
StreamState = Literal["running", "stopped", "failed"]
_TERMINAL = frozenset({"stopped", "failed"})


def crates_dir(worktree: str) -> str:
    """The cargo workspace root for otel-streams within a worktree."""
    return str(Path(worktree) / "src" / "crates")


def synth_logs_cmd(
    otel_endpoint: str, *, count: int, field_cardinality: int, spacing_nanos: int,
    start_time_nanos: int | None, seed: int, tenant_id: str | None,
    batch_size: int, flush_interval_ms: int, connect_timeout_secs: int,
    service_name: str | None = None, service_namespace: str | None = None,
) -> list[str]:
    """Argv for the one-shot LOGS synth generator (`--bin synth`)."""
    cmd = [
        "cargo", "run", "--quiet", "-p", "otel-streams", "--bin", "synth", "--",
        "--otel-endpoint", f"http://{otel_endpoint}",
        "--count", str(count),
        "--field-cardinality", str(field_cardinality),
        "--spacing-nanos", str(spacing_nanos),
        "--seed", str(seed),
        "--batch-size", str(batch_size),
        "--flush-interval-ms", str(flush_interval_ms),
        "--connect-timeout-secs", str(connect_timeout_secs),
    ]
    if start_time_nanos is not None:
        cmd += ["--start-time-nanos", str(start_time_nanos)]
    # tenant_id keeps truthiness (not `is not None`): an empty X-Scope-OrgID is an
    # invalid gRPC header value (tonic rejects it), so dropping "" is protective —
    # unlike service identity, where present-but-empty is a valid, distinct value.
    if tenant_id:
        cmd += ["--tenant-id", tenant_id]
    # service.name/namespace key the otel-ledger storage stream. Gate on
    # `is not None` (NOT truthiness) so an explicit "" is forwarded as
    # `--service-name ""` — an absent attribute (omitted → synth's own default)
    # differs from a present-but-empty one (queryable empty value), per the
    # documented OTel semantics. Omitting leaves the bin's defaults.
    if service_name is not None:
        cmd += ["--service-name", service_name]
    if service_namespace is not None:
        cmd += ["--service-namespace", service_namespace]
    return cmd


def synth_traces_cmd(
    otel_endpoint: str, *, count: int, spacing_nanos: int, duration_nanos: int,
    start_time_nanos: int | None, seed: int, tenant_id: str | None,
    batch_size: int, connect_timeout_secs: int,
    service_name: str | None = None, service_namespace: str | None = None,
) -> list[str]:
    """Argv for the one-shot TRACES synth generator (`--bin synth-traces`).

    Mirrors :func:`synth_logs_cmd` for the shared transport/identity flags. The
    traces signal has no `field_cardinality` (a logs concept) and adds
    `duration_nanos` (per-span duration). It also has no `flush_interval_ms`:
    synth-traces exports synchronously in `batch_size` chunks (no flush timer),
    so the logs `Sender`'s flush knob does not apply.
    """
    cmd = [
        "cargo", "run", "--quiet", "-p", "otel-streams", "--bin", "synth-traces", "--",
        "--otel-endpoint", f"http://{otel_endpoint}",
        "--count", str(count),
        "--spacing-nanos", str(spacing_nanos),
        "--duration-nanos", str(duration_nanos),
        "--seed", str(seed),
        "--batch-size", str(batch_size),
        "--connect-timeout-secs", str(connect_timeout_secs),
    ]
    if start_time_nanos is not None:
        cmd += ["--start-time-nanos", str(start_time_nanos)]
    # tenant_id keeps truthiness (not `is not None`): an empty X-Scope-OrgID is an
    # invalid gRPC header value (tonic rejects it), so dropping "" is protective —
    # unlike service identity, where present-but-empty is a valid, distinct value.
    if tenant_id:
        cmd += ["--tenant-id", tenant_id]
    # `is not None` (not truthiness): an explicit "" is forwarded so a
    # present-but-empty identity differs from an absent one (see synth_logs_cmd).
    if service_name is not None:
        cmd += ["--service-name", service_name]
    if service_namespace is not None:
        cmd += ["--service-namespace", service_namespace]
    return cmd


def stream_cmd(
    source: str, otel_endpoint: str, *, url: str | None, collections: str | None,
    start: str | None, rate: int | None, tenant_id: str | None,
    batch_size: int, flush_interval_ms: int,
) -> list[str]:
    """Argv for a long-running source. Only the source-appropriate optional flags
    should be passed (the tool layer rejects mismatched ones)."""
    cmd = [
        "cargo", "run", "--quiet", "-p", "otel-streams", "--bin", source, "--",
        "--otel-endpoint", f"http://{otel_endpoint}",
        "--batch-size", str(batch_size),
        "--flush-interval-ms", str(flush_interval_ms),
    ]
    if tenant_id:
        cmd += ["--tenant-id", tenant_id]
    # `url` is guarded but collections/start/rate are not — by design, not
    # oversight: url's FLAG NAME is source-dependent (--certstream-url vs
    # --jetstream-url), so a wrong source would silently emit a plausible-but-
    # wrong flag; the explicit check prevents that. The others have fixed flag
    # names, so a mismatch is just an unknown flag the bin rejects loudly.
    # (validate_source_params rejects all mismatches upstream regardless.)
    if url:
        if source not in ("certstream", "jetstream"):
            raise ValueError(f"url is only valid for certstream/jetstream, not {source!r}")
        cmd += ["--certstream-url" if source == "certstream" else "--jetstream-url", url]
    if collections:  # jetstream only
        cmd += ["--collections", collections]
    if start:  # github only
        cmd += ["--start", start]
    if rate is not None:  # github only
        cmd += ["--rate", str(rate)]
    return cmd


async def run_synth(worktree: str, cmd: list[str], *, timeout: int) -> tuple[int | None, str, str | None]:
    """Run a one-shot generator ``cmd`` (synth or synth-traces) to completion in
    the worktree's crates dir. Returns ``(returncode, log_tail, error)`` and does
    not raise on a timeout or spawn failure. ``rc == 0`` is authoritative
    end-to-end success: both generator binaries ``bail!`` (non-zero) if any batch
    failed to export, so the caller needn't second-guess a zero exit.

    On timeout — or on outer cancellation — the whole process group is killed so
    the shielded task can't leave a cargo/generator child running unobserved.
    """
    buffer = LogBuffer()
    holder: dict[str, asyncio.subprocess.Process] = {}
    task = asyncio.get_running_loop().create_task(
        run_command(cmd, crates_dir(worktree), buffer.append, on_spawn=lambda p: holder.__setitem__("p", p))
    )
    try:
        rc = await asyncio.wait_for(asyncio.shield(task), timeout=timeout)
    except asyncio.TimeoutError:
        kill_process_group(holder.get("p"), signal.SIGKILL)
        try:
            await task  # rejoin the killed task; proc is gone (no-op) or reaped
        except BaseException:
            pass
        return None, buffer.tail(20), f"generator timed out after {timeout}s (first run also builds it)"
    except asyncio.CancelledError:
        # Outer cancel (e.g. client disconnect on http transport): shield kept
        # the task — and its cargo/synth process group — alive, so tear it down
        # before propagating (CancelledError is a BaseException and bypasses the
        # `except Exception` below).
        proc = holder.get("p")
        if proc is not None:
            kill_process_group(proc, signal.SIGKILL)
        else:
            task.cancel()  # not spawned yet → cancel run_command so its finally cleans up
        try:
            await task
        except BaseException:
            pass
        raise
    except Exception as exc:  # spawn failure (e.g. cargo not found)
        return None, buffer.tail(20), f"failed to run generator: {exc!r}"
    return rc, buffer.tail(20), None


@dataclass
class Stream:
    stream_id: str
    agent_id: str
    source: str
    otel_endpoint: str
    buffer: LogBuffer = field(default_factory=LogBuffer)
    state: StreamState = "running"
    error: str | None = None
    returncode: int | None = None
    created_at: float = field(default_factory=time.monotonic)

    _proc: asyncio.subprocess.Process | None = field(default=None, repr=False)
    _task: asyncio.Task | None = field(default=None, repr=False)
    _cancelled: bool = field(default=False, repr=False)

    @property
    def done(self) -> bool:
        return self.state in _TERMINAL

    def elapsed(self) -> float:
        return time.monotonic() - self.created_at

    def _set_proc(self, proc: asyncio.subprocess.Process) -> None:
        self._proc = proc

    def request_cancel(self) -> None:
        self._cancelled = True
        kill_process_group(self._proc, signal.SIGTERM)

    def force_kill(self) -> None:
        kill_process_group(self._proc, signal.SIGKILL)


class StreamRegistry:
    """In-memory registry of long-running otel-streams daemons (no persistence,
    no eviction — a localhost dev tool). Mirrors ``RunRegistry``'s lifecycle."""

    def __init__(self) -> None:
        self._streams: dict[str, Stream] = {}
        self._counter = itertools.count(1)

    def get(self, stream_id: str) -> Stream | None:
        return self._streams.get(stream_id)

    def list(self) -> list[Stream]:
        return list(self._streams.values())

    def start(self, agent_id: str, worktree: str, otel_endpoint: str, source: str, cmd: list[str]) -> Stream:
        stream_id = f"{agent_id}:{source}:{next(self._counter)}"
        stream = Stream(stream_id=stream_id, agent_id=agent_id, source=source, otel_endpoint=otel_endpoint)
        self._streams[stream_id] = stream
        stream._task = asyncio.get_running_loop().create_task(self._drive(stream, worktree, cmd))
        return stream

    async def _drive(self, stream: Stream, worktree: str, cmd: list[str]) -> None:
        try:
            stream.buffer.append(f"[stream {stream.source} -> {stream.otel_endpoint}] {' '.join(cmd)}")
            rc = await run_command(cmd, crates_dir(worktree), stream.buffer.append, on_spawn=stream._set_proc)
            stream.returncode = rc
            # A daemon that exits on its own is a failure unless we asked it to stop.
            stream.state = "stopped" if stream._cancelled else "failed"
            if not stream._cancelled:
                stream.error = f"stream exited unexpectedly (code {rc})"
        except Exception as exc:  # surface, never crash the server
            stream.error = str(exc)
            stream.state = "stopped" if stream._cancelled else "failed"
            stream.buffer.append(f"[error: {exc}]")
        finally:
            stream._proc = None  # process is gone on every path; drop the handle (mirrors Run)
            if stream.state not in _TERMINAL:  # cancelled task bypasses except
                stream.state = "stopped"

    async def stop(self, stream_id: str, *, wait: float = 5.0) -> Stream | None:
        stream = self._streams.get(stream_id)
        if stream is None:
            return None
        await escalate_cancel(stream, wait=wait)
        return stream

    async def stop_all(self, *, wait: float = 10.0) -> None:
        await drain_all(self._streams.values(), wait=wait)
