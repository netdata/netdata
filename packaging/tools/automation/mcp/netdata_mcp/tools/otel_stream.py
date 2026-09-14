"""Otel streams: run a continuous real-world OTLP source against an agent.

The live otel-streams sources (certstream / jetstream / github) are daemons —
they never exit on their own — so they get a start/status/stop lifecycle (like a
launched agent), unlike the one-shot `netdata_agent_otel_push_logs`. One source-enum
trio, not a tool per source: the lifecycle is identical across them; only a few
optional params differ, and those are rejected when they don't match the source.
"""

from __future__ import annotations

from typing import Annotated, Literal

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, Field

from .. import streams
from ._common import get_runs, get_streams

_Source = Annotated[
    Literal["certstream", "jetstream", "github"],
    Field(description="Which live source to run: certstream (CT logs), jetstream (Bluesky firehose), github (GH Archive replay)."),
]
_AgentId = Annotated[str, Field(description="A ready agent (declare + run_start) to feed.")]
_StreamId = Annotated[str, Field(description="Stream id returned by netdata_agent_otel_stream_start.")]
_Url = Annotated[str | None, Field(description="Override source URL — certstream (--certstream-url) or jetstream (--jetstream-url) only.")]
_Collections = Annotated[str | None, Field(description="jetstream only: comma-separated collection filter.")]
_Start = Annotated[str | None, Field(description="github only: archive start hour, YYYY-MM-DD-H.")]
_Rate = Annotated[int | None, Field(description="github only: replay rate (events/s; 0 = unlimited).", ge=0)]
_Tenant = Annotated[str | None, Field(description="Tenant id sent via the X-Scope-OrgID gRPC header.")]
_BatchSize = Annotated[int, Field(description="Max records per gRPC export request.", ge=1)]
_FlushMs = Annotated[int, Field(description="Max ms before flushing a partial batch.", ge=1)]


class StreamInfo(BaseModel):
    stream_id: str | None = None
    agent_id: str = ""
    source: str | None = None
    otel_endpoint: str | None = None
    # The registry's Stream.state is Literal[running|stopped|failed]; this wire
    # model also carries response-only states (unknown = no such stream, error =
    # bad request) that never exist in the registry, so it stays a free str.
    state: str = Field(description="running | stopped | failed | unknown | error")
    returncode: int | None = None
    log_tail: str | None = None
    error: str | None = None
    message: str = ""


class StreamList(BaseModel):
    streams: list[StreamInfo] = Field(default_factory=list, description="All streams the server has started (running + terminal).")


def _info(s: streams.Stream, *, message: str = "") -> StreamInfo:
    return StreamInfo(
        stream_id=s.stream_id, agent_id=s.agent_id, source=s.source,
        otel_endpoint=s.otel_endpoint, state=s.state, returncode=s.returncode,
        log_tail=s.buffer.tail(20), error=s.error, message=message,
    )


def validate_source_params(
    source: str, *, url: str | None, collections: str | None, start: str | None, rate: int | None
) -> str | None:
    """Reject params that don't apply to the chosen source, so a misdirected flag
    fails clearly instead of being silently ignored."""
    bad = []
    if url is not None and source not in ("certstream", "jetstream"):
        bad.append("url (certstream/jetstream only)")
    if collections is not None and source != "jetstream":
        bad.append("collections (jetstream only)")
    if start is not None and source != "github":
        bad.append("start (github only)")
    if rate is not None and source != "github":
        bad.append("rate (github only)")
    if bad:
        return f"params not valid for source {source!r}: {', '.join(bad)}"
    return None


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_stream_start",
        description=(
            "Start a continuous real-world OTLP source feeding a ready agent's otel "
            "plugin (certstream/jetstream/github). Returns a stream_id; the source runs "
            "until netdata_agent_otel_stream_stop. For a deterministic one-shot corpus "
            "use netdata_agent_otel_push_logs / netdata_agent_otel_push_traces instead. "
            "First call also builds the source (cargo)."
        ),
    )
    async def netdata_agent_otel_stream_start(
        ctx: Context,
        agent_id: _AgentId,
        source: _Source,
        url: _Url = None,
        collections: _Collections = None,
        start: _Start = None,
        rate: _Rate = None,
        tenant_id: _Tenant = None,
        batch_size: _BatchSize = 100,
        flush_interval_ms: _FlushMs = 1000,
    ) -> StreamInfo:
        run = get_runs(ctx).get(agent_id)
        if run is None or run.state != "ready" or not run.otlp_endpoint:
            state = run.state if run is not None else "unknown"
            return StreamInfo(
                agent_id=agent_id, state="error",
                error=f"Agent {agent_id!r} is not ready (state={state}) or has no OTLP endpoint. "
                "Start it with netdata_run_start and poll until 'ready'.",
            )
        verr = validate_source_params(source, url=url, collections=collections, start=start, rate=rate)
        if verr is not None:
            return StreamInfo(agent_id=agent_id, source=source, state="error", error=verr)
        cmd = streams.stream_cmd(
            source, run.otlp_endpoint, url=url, collections=collections, start=start,
            rate=rate, tenant_id=tenant_id, batch_size=batch_size, flush_interval_ms=flush_interval_ms,
        )
        s = get_streams(ctx).start(agent_id, run.worktree, run.otlp_endpoint, source, cmd)
        return _info(s, message=f"stream {source} started; poll netdata_agent_otel_stream_status({s.stream_id!r})")

    @mcp.tool(
        name="netdata_agent_otel_stream_status",
        description="Status of a running otel stream (running|stopped|failed) plus its recent output.",
    )
    async def netdata_agent_otel_stream_status(ctx: Context, stream_id: _StreamId) -> StreamInfo:
        s = get_streams(ctx).get(stream_id)
        if s is None:
            return StreamInfo(state="unknown", error=f"No such stream {stream_id!r}.")
        return _info(s)

    @mcp.tool(
        name="netdata_agent_otel_stream_stop",
        description="Stop a running otel stream (SIGTERM→SIGKILL its process group). Returns the final state.",
    )
    async def netdata_agent_otel_stream_stop(ctx: Context, stream_id: _StreamId) -> StreamInfo:
        s = await get_streams(ctx).stop(stream_id)
        if s is None:
            return StreamInfo(state="unknown", error=f"No such stream {stream_id!r}.")
        # The stream may have already exited on its own before we stopped it.
        msg = "stream stopped" if s.state == "stopped" else f"stream was already {s.state}"
        return _info(s, message=msg)

    @mcp.tool(
        name="netdata_agent_otel_stream_list",
        description="List all otel streams this server has started (running and terminal), with state.",
    )
    async def netdata_agent_otel_stream_list(ctx: Context) -> StreamList:
        return StreamList(streams=[_info(s) for s in get_streams(ctx).list()])
