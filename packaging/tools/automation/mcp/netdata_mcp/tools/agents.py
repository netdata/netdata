"""Agents domain: declare an agent (id -> worktree + profile)."""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .. import buildcfg
from ._common import Profile, _Worktree, get_agents, get_runs
from .models import RunInfo, agent_declared, agent_error, run_info

_AgentId = Annotated[str, Field(description="A short handle for this agent (letters/digits/_/-), reused across run tools.")]
_Profile = Annotated[Profile, Field(description="Build profile this agent runs.")]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_declare",
        description=(
            "Declare an agent: bind an agent-id to a (worktree, profile). Idempotent "
            "(re-declaring updates it). Then drive it by id with netdata_run_start / "
            "_status / _logs / _stop. The build behind it is the worktree's single "
            "build/ (the profile sets its build type); the agent is its own isolated "
            "run instance."
        ),
    )
    async def netdata_agent_declare(ctx: Context, agent_id: _AgentId, worktree: _Worktree, profile: _Profile) -> RunInfo:
        if not buildcfg.is_worktree(worktree):
            return agent_error(agent_id, f"Not a Netdata worktree (no CMakeLists.txt): {worktree}")
        live = get_runs(ctx).get(agent_id)
        if live is not None and not live.done and (live.worktree != worktree or live.profile != profile):
            return agent_error(
                agent_id,
                f"Agent {agent_id!r} is running ({live.profile} @ {live.worktree}); "
                f"stop it (netdata_run_stop) before re-declaring with a different worktree/profile.",
            )
        try:
            spec = get_agents(ctx).declare(agent_id, worktree, profile)
        except ValueError as exc:
            return agent_error(agent_id, str(exc))
        if live is not None and not live.done:
            # same (worktree, profile), still running — reflect the live state
            # rather than misreporting "declared".
            return run_info(live, message=f"Agent {agent_id!r} already running; state preserved.")
        return agent_declared(spec, message=f"Agent {agent_id!r} declared ({profile} @ {worktree}). Start it with netdata_run_start.")
