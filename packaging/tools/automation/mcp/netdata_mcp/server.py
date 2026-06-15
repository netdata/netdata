#!/usr/bin/env python3

"""MCP server for configuring, building, and running Netdata from a worktree.

Surface (fire-and-poll; no synchronous tool):
  build/configure:
    - netdata_configure_start(worktree, profile) -> job_id
    - netdata_build_start(worktree, profile)     -> job_id  (configures first if needed)
    - netdata_job_status(job_id)                 -> long-polls until terminal
    - netdata_job_logs(job_id, offset)           -> incremental output
    - netdata_job_cancel(job_id)
  run (by agent-id):
    - netdata_agent_declare(agent_id, worktree, profile)
    - netdata_run_start(agent_id, restart=False) -> build+install if needed, then launch
                                                    (restart=True: stop + rebuild + relaunch)
    - netdata_run_status(agent_id)               -> building|starting|ready|stopped|failed
    - netdata_run_logs(agent_id, offset)
    - netdata_run_stop(agent_id)

Localhost-only: there is no authentication and no path sandboxing — it will run
cmake/ninja/netdata against any worktree path a client provides. Do not expose it.
"""

from __future__ import annotations

import argparse
from contextlib import asynccontextmanager
from dataclasses import dataclass

from mcp.server.fastmcp import FastMCP

from .agents import AgentRegistry
from .jobs import JobRegistry
from .run import RunRegistry
from .streams import StreamRegistry
from .tools import (
    agent_mcp,
    agents,
    build,
    configure,
    job_control,
    otel_config,
    otel_logs,
    otel_push,
    otel_stream,
    run,
)

_INSTRUCTIONS = (
    "Configure, build, and run the Netdata Agent from a local worktree. "
    "Build: *_start tools return a job_id immediately; poll netdata_job_status until "
    "it's no longer 'running'. Run: declare an agent (netdata_agent_declare), then "
    "netdata_run_start/_status/_logs/_stop by agent-id; netdata_run_start builds+installs "
    "if needed and launches netdata on an auto-assigned port — poll until 'ready'. "
    "Builds go in <worktree>/build/ (one per worktree; use a worktree dedicated to LLM runs); "
    "clangd finds build/compile_commands.json there natively, so editor/clangd errors that "
    "contradict a successful build are stale-database false positives — trust the build. "
    "Query a ready agent: netdata_agent_<name> tools (e.g. netdata_agent_query_metrics) "
    "forward to that agent's own /mcp by agent_id, to verify what you just built. "
    "Localhost-only dev tool."
)


@dataclass
class AppContext:
    registry: JobRegistry
    agents: AgentRegistry
    runs: RunRegistry
    streams: StreamRegistry


@asynccontextmanager
async def _lifespan(_server: FastMCP):
    registry = JobRegistry()
    agent_registry = AgentRegistry()
    runs = RunRegistry()
    streams = StreamRegistry()
    try:
        yield AppContext(registry=registry, agents=agent_registry, runs=runs, streams=streams)
    finally:
        # Don't leave cmake/ninja/netdata/cargo children running after the server stops.
        await streams.stop_all()
        await runs.stop_all()
        await registry.cancel_all()


def build_server() -> FastMCP:
    mcp = FastMCP(
        "netdata-build-mcp",
        instructions=_INSTRUCTIONS,
        # Stateful sessions: the job registry lives in the app lifespan and must
        # persist across requests so a client can start a job and then poll it.
        # stateless_http=True gives each request a fresh lifespan context (and
        # thus a fresh, empty registry), which loses jobs between calls.
        stateless_http=False,
        lifespan=_lifespan,
    )
    job_control.register(mcp)
    configure.register(mcp)
    build.register(mcp)
    agents.register(mcp)
    run.register(mcp)
    otel_config.register(mcp)
    otel_logs.register(mcp)
    otel_push.register(mcp)
    otel_stream.register(mcp)
    agent_mcp.register(mcp)
    return mcp


mcp = build_server()


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        prog="netdata-build-mcp",
        description="MCP server to configure, build, and run Netdata from a worktree.",
    )
    p.add_argument(
        "--transport",
        choices=("stdio", "http"),
        default="stdio",
        help=(
            "stdio (default): the client spawns the server; agents are stopped when the "
            "client session ends. http: streamable-HTTP; the server runs independently and "
            "agents/builds survive client restarts (run it yourself, point clients at the URL)."
        ),
    )
    p.add_argument("--host", default="127.0.0.1", help="bind host for --transport http (default 127.0.0.1)")
    p.add_argument("--port", type=int, default=8000, help="bind port for --transport http (default 8000)")
    return p.parse_args(argv)


def main() -> None:
    args = parse_args()
    if args.transport == "http":
        mcp.settings.host = args.host
        mcp.settings.port = args.port
        mcp.run(transport="streamable-http")
    else:
        mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
