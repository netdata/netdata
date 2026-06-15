"""Run domain: launch / inspect / stop a declared agent, by agent-id."""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from ._common import get_agents, get_runs
from .models import RunInfo, RunLogs, run_info, unknown_agent

_AgentId = Annotated[str, Field(description="Agent id from netdata_agent_declare.")]
_Restart = Annotated[
    bool,
    Field(
        description=(
            "If the agent is already running, stop it, rebuild (incremental), and relaunch "
            "to pick up source changes. Default False is idempotent: a live agent is returned "
            "unchanged and nothing is rebuilt. Use restart=true after editing source."
        ),
    ),
]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_run_start",
        description=(
            "Start a declared agent: build+install its (worktree, profile) if needed, "
            "then launch netdata on an auto-assigned loopback port. Returns immediately; "
            "poll netdata_run_status until state is 'ready'. First start for a profile "
            "builds+installs and can take minutes. Idempotent while the agent is live: a "
            "plain start does NOT rebuild a running agent. After editing source, pass "
            "restart=true (stop + incremental rebuild + relaunch) to apply the changes."
        ),
    )
    async def netdata_run_start(ctx: Context, agent_id: _AgentId, restart: _Restart = False) -> RunInfo:
        spec = get_agents(ctx).get(agent_id)
        if spec is None:
            return unknown_agent(agent_id)
        run, outcome = await get_runs(ctx).start(
            agent_id, spec.worktree, spec.profile, otel=spec.otel, restart=restart
        )
        poll = f"Poll netdata_run_status({agent_id!r}) until 'ready'."
        # "already-running" covers any live state; say "running" only when ready.
        live = "running" if run.state == "ready" else "starting up"
        messages = {
            "started": f"Starting agent {agent_id!r}. {poll}",
            "restarted": f"Restarting agent {agent_id!r}: rebuild + relaunch. {poll}",
            "already-running": (
                f"Agent {agent_id!r} is already {live} (unchanged) — nothing was rebuilt. "
                "To apply source changes, call netdata_run_start with restart=true."
            ),
        }
        return run_info(run, message=messages[outcome])

    @mcp.tool(
        name="netdata_run_status",
        description=(
            "Status of a running agent: building | starting | ready | stopped | failed, "
            "plus its port and url when ready, and (once ready) claimed / cloud_connected "
            "for its Netdata Cloud status. Long-polls ~8s while still coming up; call "
            "repeatedly until 'ready' (or 'failed'/'stopped')."
        ),
    )
    async def netdata_run_status(ctx: Context, agent_id: _AgentId) -> RunInfo:
        run = await get_runs(ctx).wait_status(agent_id)
        if run is None:
            return unknown_agent(agent_id)
        return run_info(run)

    @mcp.tool(
        name="netdata_run_logs",
        description="Fetch an agent's combined build + netdata output incrementally (pass back next_offset).",
    )
    async def netdata_run_logs(
        ctx: Context,
        agent_id: _AgentId,
        offset: Annotated[int, Field(description="Line offset from a prior call; 0 to start.", ge=0)] = 0,
    ) -> RunLogs:
        run = get_runs(ctx).get(agent_id)
        if run is None:
            return RunLogs(agent_id=agent_id, state="unknown", text="", next_offset=0, truncated=False,
                           message="No such agent (in-memory; does not survive a server restart).")
        sl = run.buffer.read(offset)
        return RunLogs(agent_id=agent_id, state=run.state, text=sl.text, next_offset=sl.next_offset,
                       truncated=sl.truncated, message=("Some earlier lines were evicted." if sl.truncated else ""))

    @mcp.tool(
        name="netdata_run_stop",
        description="Stop a running agent (terminates its process group). Returns the final state.",
    )
    async def netdata_run_stop(ctx: Context, agent_id: _AgentId) -> RunInfo:
        run = await get_runs(ctx).stop(agent_id)
        if run is None:
            return unknown_agent(agent_id)
        return run_info(run, message=f"Agent {agent_id!r} {run.state}.")
