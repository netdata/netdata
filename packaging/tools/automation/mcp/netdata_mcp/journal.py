"""systemd-journal access for agent diagnostics.

When an agent is launched with ``[logs] collector = journal`` (the default on
journald hosts — :func:`runtime._default_conf` writes ``stderr`` instead when no
journald socket is present), the otel-plugin workers log via ``tracing-journald``
under ``SYSLOG_IDENTIFIER=otel-plugin/<worker>``.

A single SYSLOG_IDENTIFIER is shared across every agent, so to scope a query to
one agent we filter journalctl by the worker's ``_PID``. The supervisor spawns
each worker as ``<otel-plugin> worker <name> --socket <path>`` (a descendant of
the agent's netdata process) and never restarts it, so the PID is stable for the
run's lifetime. We resolve it from ``/proc`` (pure stdlib — no extra dep).
"""

from __future__ import annotations

import asyncio
import contextlib
import os
import shutil
from pathlib import Path

# Presence of this socket means systemd-journald is running and accepting logs.
_JOURNALD_SOCKET = Path("/run/systemd/journal/socket")


def journald_socket_present() -> bool:
    """Whether the systemd-journald socket exists on this host.

    Socket presence (not a liveness probe): systemd removes the socket when
    journald stops, so on a normal host this tracks "journald is up", but a stale
    socket from an unclean shutdown would still read True. A read against a dead
    socket degrades gracefully in :func:`read_logs`, so that edge is harmless. We
    swallow ``OSError`` (e.g. EACCES on a hardened ``/run/systemd/journal``) and
    treat it as absent — this is called at server startup, so raising would abort
    boot rather than just hiding the tool.
    """
    try:
        return _JOURNALD_SOCKET.exists()
    except OSError:
        return False


def usable() -> bool:
    """Whether the journal is usable for agent diagnostics here.

    Requires both that agents can log to it (journald socket present) and that we
    can read it back (``journalctl`` on PATH). When false there is nothing for
    :func:`read_logs` to query, so the ``netdata_agent_logs`` tool isn't exposed.
    """
    return journald_socket_present() and shutil.which("journalctl") is not None


# Max wall-clock for a single journalctl invocation. Bounds a hung query
# (stalled journald, slow disk, a pathological --grep regex) — matches the
# wait_for-bounded subprocess pattern used elsewhere in the server.
READ_TIMEOUT = 30.0

# Log sources, in process-tree order: the netdata daemon, the otel-plugin
# supervisor, then its three workers. The worker name matches both the
# SYSLOG_IDENTIFIER suffix and the `worker <name>` spawn arg (otel-plugin
# supervisor.rs / main.rs).
WORKER_COMPONENTS = ("ledger", "ingestor", "legacy-logs")
SOURCES = ("daemon", "supervisor", *WORKER_COMPONENTS)


def syslog_identifier(source: str) -> str:
    """The journal SYSLOG_IDENTIFIER for a log source."""
    if source == "daemon":
        return "netdata"
    if source == "supervisor":
        return "otel-plugin"
    return f"otel-plugin/{source}"


def _parse_ppid(stat: str) -> int | None:
    """Parent PID from the contents of ``/proc/<pid>/stat``, or None.

    Format: ``pid (comm) state ppid ...``. ``comm`` may contain spaces, so parse
    the fields after the LAST ``')'``: index 0 = state, index 1 = ppid.
    """
    rparen = stat.rfind(")")
    if rparen < 0:
        return None
    fields = stat[rparen + 1 :].split()
    if len(fields) < 2:
        return None
    try:
        return int(fields[1])
    except ValueError:
        return None


def _ppid_of(pid: int) -> int | None:
    """Parent PID from ``/proc/<pid>/stat``, or None."""
    try:
        with open(f"/proc/{pid}/stat", encoding="utf-8", errors="replace") as fh:
            return _parse_ppid(fh.read())
    except OSError:
        return None


def _descendant_pids(root_pid: int) -> set[int]:
    """All descendant PIDs of ``root_pid`` (excluding it)."""
    children: dict[int, list[int]] = {}
    try:
        names = os.listdir("/proc")
    except OSError:
        return set()
    for name in names:
        if not name.isdigit():
            continue
        pid = int(name)
        ppid = _ppid_of(pid)
        if ppid is not None:
            children.setdefault(ppid, []).append(pid)

    out: set[int] = set()
    stack = [root_pid]
    while stack:
        for child in children.get(stack.pop(), []):
            if child not in out:
                out.add(child)
                stack.append(child)
    return out


def _cmdline(pid: int) -> list[str]:
    """Argv of ``pid`` from ``/proc/<pid>/cmdline`` (NUL-separated)."""
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as fh:
            raw = fh.read()
    except OSError:
        return []
    return [part.decode("utf-8", "replace") for part in raw.split(b"\0") if part]


def _find_descendant(netdata_pid: int, predicate) -> int | None:
    for pid in _descendant_pids(netdata_pid):
        parts = _cmdline(pid)
        if parts and predicate(parts):
            return pid
    return None


def resolve_pid(netdata_pid: int, source: str) -> int | None:
    """PID to scope a journal query to, for ``source`` under ``netdata_pid``.

    - ``daemon``: netdata itself (the MCP already tracks this PID).
    - ``supervisor``: the otel-plugin process (a child of netdata whose argv is
      NOT ``worker ...``).
    - a worker: the descendant whose argv is ``<bin> worker <source> ...``.

    Workers and the supervisor are never restarted, so a resolved PID is stable
    for the run. Returns None if not found (e.g. agent not running).
    """
    if source == "daemon":
        return netdata_pid
    if source == "supervisor":
        return _find_descendant(
            netdata_pid,
            lambda a: a[0].rsplit("/", 1)[-1] == "otel-plugin" and (len(a) < 2 or a[1] != "worker"),
        )
    return _find_descendant(
        netdata_pid,
        lambda a: len(a) >= 3 and a[1] == "worker" and a[2] == source,
    )


async def read_logs(
    *,
    identifier: str,
    pid: int | None,
    lines: int,
    grep: str | None,
    priority: str | None,
) -> str:
    """Run ``journalctl`` read-only and return its combined output.

    Scopes to ``SYSLOG_IDENTIFIER=<identifier>`` and, when ``pid`` is given, to
    that process. Field matches and options may be freely mixed.
    """
    cmd = [
        "journalctl",
        "-n", str(lines),
        "-o", "short-iso",
        "--no-pager",
    ]
    if priority:
        cmd += ["-p", priority]
    if grep:
        cmd += ["--grep", grep]
    cmd.append(f"SYSLOG_IDENTIFIER={identifier}")
    if pid is not None:
        cmd.append(f"_PID={pid}")

    # Never raise: this backs an MCP tool whose contract is a structured result,
    # not an exception. Surface failures (missing binary, timeout, non-zero exit)
    # as text instead. `start_new_session` so a timeout kill targets only
    # journalctl, not the server's process group.
    try:
        proc = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            start_new_session=True,
        )
    except FileNotFoundError:
        return "[journalctl not found on this host]"
    except OSError as exc:
        return f"[failed to run journalctl: {exc}]"

    try:
        out, _ = await asyncio.wait_for(proc.communicate(), timeout=READ_TIMEOUT)
    except asyncio.TimeoutError:
        # The process may have exited between the timeout and the kill — never raise.
        with contextlib.suppress(ProcessLookupError):
            proc.kill()
        # communicate() (not wait()) so the pipe transports are drained/closed.
        with contextlib.suppress(OSError):
            await proc.communicate()
        return f"[journalctl timed out after {READ_TIMEOUT:.0f}s]"

    text = out.decode("utf-8", "replace")
    if proc.returncode:
        text = f"[journalctl exited {proc.returncode}]\n{text}"
    return text
