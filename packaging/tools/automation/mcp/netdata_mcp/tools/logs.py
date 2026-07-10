"""Agent diagnostics: scoped otel-plugin worker logs from the systemd journal."""

from __future__ import annotations

import asyncio
from typing import Annotated, Literal

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .. import journal
from ._common import get_runs
from .models import AgentLogs, unknown_agent_logs

_AgentId = Annotated[str, Field(description="Agent id from netdata_agent_declare.")]
_Component = Annotated[
    Literal["daemon", "supervisor", "ledger", "ingestor", "legacy-logs"],
    Field(
        description=(
            "Which part of the agent: 'daemon' (netdata itself), 'supervisor' "
            "(the otel-plugin process), or a specific otel-plugin worker "
            "('ledger', 'ingestor', 'legacy-logs')."
        ),
    ),
]
_Lines = Annotated[int, Field(description="Tail this many journal entries.", ge=1, le=5000)]
_Grep = Annotated[
    str | None,
    Field(
        description=(
            "journalctl --grep regex (PERL-compatible); matches the message text. "
            "Requires systemd >= 240."
        ),
    ),
]
_Priority = Annotated[
    Literal["emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"] | None,
    Field(
        description=(
            "journalctl -p threshold: returns entries at this level OR MORE SEVERE "
            "(e.g. 'err' => err+crit+alert+emerg). Omit for all levels."
        )
    ),
]


def _resolution_note(netdata_pid: int | None, pid: int | None, component: str) -> str:
    """Explain why a query is/ isn't PID-scoped, for the AgentLogs.message field."""
    if pid is not None:
        return ""
    if netdata_pid is None:
        return (
            "Agent not running. Results are identifier-scoped and may span "
            "other agents or be empty."
        )
    return (
        f"Agent is running but its '{component}' process was not found in /proc "
        "(not started yet, or it exited). Results are identifier-scoped and may "
        "span other agents."
    )


def register(mcp: FastMCP) -> None:
    # Only expose the tool where the journal is actually usable (journalctl on
    # PATH + a running journald). Off systemd, agents log to stderr/files instead,
    # so this tool would have nothing to read — leaving it out steers callers to
    # netdata_run_logs / the run-dir log files.
    if not journal.usable():
        return

    @mcp.tool(
        name="netdata_agent_logs",
        description=(
            "Fetch an agent's logs from the systemd journal: the netdata daemon "
            "('daemon', the default), the otel-plugin supervisor ('supervisor'), or an "
            "otel-plugin worker ('ledger'/'ingestor'/'legacy-logs'). When the agent is "
            "running, the query is scoped to that process via _PID so it isn't mixed with "
            "other agents (the daemon identifier 'netdata' especially is shared host-wide). "
            "Read-only. Prefer this over netdata_run_logs when you want structured, "
            "per-component logs with grep/priority filtering. Only registered on journald "
            "hosts; elsewhere the agent logs to stderr — use netdata_run_logs there."
        ),
    )
    async def netdata_agent_logs(
        ctx: Context,
        agent_id: _AgentId,
        component: _Component = "daemon",
        lines: _Lines = 200,
        grep: _Grep = None,
        priority: _Priority = None,
    ) -> AgentLogs:
        run = get_runs(ctx).get(agent_id)
        identifier = journal.syslog_identifier(component)
        if run is None:
            return unknown_agent_logs(agent_id, component, identifier)

        # Scope to this agent's process when it's alive; otherwise fall back to
        # identifier-only (historical entries, possibly across agents). The /proc
        # walk is synchronous, so run it off the event loop.
        netdata_pid = run.pid
        pid = (
            await asyncio.to_thread(journal.resolve_pid, netdata_pid, component)
            if netdata_pid is not None
            else None
        )
        text = await journal.read_logs(
            identifier=identifier, pid=pid, lines=lines, grep=grep, priority=priority
        )
        note = _resolution_note(netdata_pid, pid, component)
        return AgentLogs(
            agent_id=agent_id,
            component=component,
            syslog_identifier=identifier,
            pid=pid,
            text=text,
            message=note,
        )
