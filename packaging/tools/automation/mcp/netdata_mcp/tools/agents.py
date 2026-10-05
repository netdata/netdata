"""Agents domain: declare an agent (id -> worktree + profile)."""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .. import buildcfg, runtime
from ._common import Profile, _Worktree, get_agents, get_runs
from .models import RunInfo, agent_declared, agent_error, run_info

_AgentId = Annotated[str, Field(description="A short handle for this agent (letters/digits/_/-), reused across run tools.")]
_Profile = Annotated[Profile, Field(description="Build profile this agent runs.")]
_Port = Annotated[
    int | None,
    Field(
        ge=1,
        le=65535,
        description=(
            "Optional fixed web port (127.0.0.1) for every start/restart of this agent, "
            "so its URL stays the same. Omit for an auto-assigned port per start. A start "
            "fails if the port is already in use. Refused if a pinned OTLP endpoint "
            "(netdata_agent_otel_config) uses it on 127.0.0.1 or a wildcard address."
        ),
    ),
]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_declare",
        description=(
            "Declare an agent: bind an agent-id to a (worktree, profile). Idempotent "
            "(re-declaring updates it). Then drive it by id with netdata_run_start / "
            "_status / _logs / _stop. The build behind it is the worktree's single "
            "build/ (the profile sets its build type); the agent is its own isolated "
            "run instance. Pass port to pin its web port across restarts (re-declaring "
            "without port returns it to auto-assigned ports; a change applies at the next "
            "start/restart)."
        ),
    )
    async def netdata_agent_declare(
        ctx: Context, agent_id: _AgentId, worktree: _Worktree, profile: _Profile, port: _Port = None
    ) -> RunInfo:
        if not buildcfg.is_worktree(worktree):
            return agent_error(agent_id, f"Not a Netdata worktree (no CMakeLists.txt): {worktree}")
        live = get_runs(ctx).get(agent_id)
        if live is not None and not live.done and (live.worktree != worktree or live.profile != profile):
            return agent_error(
                agent_id,
                f"Agent {agent_id!r} is running ({live.profile} @ {live.worktree}); "
                f"stop it (netdata_run_stop) before re-declaring with a different worktree/profile.",
            )
        known = get_agents(ctx).get(agent_id)
        if (clash := runtime.web_port_clash(known.otel if known else None, port)) is not None:
            return agent_error(agent_id, clash)
        try:
            spec = get_agents(ctx).declare(agent_id, worktree, profile, port=port)
        except ValueError as exc:
            return agent_error(agent_id, str(exc))
        if live is not None and not live.done:
            # same (worktree, profile), still running — reflect the live state
            # rather than misreporting "declared".
            pending = (
                f" It keeps port {live.port} until netdata_run_start(restart=true)."
                if (port is not None and port != live.port) or (port is None and live.port_pinned)
                else ""
            )
            return run_info(live, message=f"Agent {agent_id!r} already running; state preserved.{pending}")
        where = f" on port {port}" if port is not None else ""
        return agent_declared(
            spec, message=f"Agent {agent_id!r} declared ({profile} @ {worktree}){where}. Start it with netdata_run_start."
        )
