"""Otel push: send a deterministic synthetic OTLP corpus to a ready agent.

One-shot, synchronous: it runs the `otel-streams synth` generator to completion
against the agent's OTLP endpoint and returns the outcome. Pair with
`netdata_agent_otel_config` (tiny rotation thresholds) + `netdata_agent_otel_logs`
to verify storage edge cases over a known-exact corpus. For continuous real-world
input use the `netdata_agent_otel_stream_*` tools instead.
"""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, Field

from .. import streams
from ._common import get_runs

_AgentId = Annotated[str, Field(description="A ready agent (declare + run_start) to push logs to.")]
_Count = Annotated[int, Field(description="Number of synthetic log records to send.", ge=1, le=10_000_000)]
_FieldCard = Annotated[int, Field(description="Distinct values per mid-cardinality attribute (host, code).", ge=1)]
_Spacing = Annotated[int, Field(description="Nanoseconds between consecutive records.", ge=0)]
_StartTime = Annotated[int | None, Field(description="First record timestamp (unix nanos). Default: now − count·spacing, so the batch lands in the recent past.", ge=0)]
_Seed = Annotated[int, Field(description="Deterministic value-selection offset; seeds within [0, field_cardinality) give distinct corpora.", ge=0)]
_ServiceName = Annotated[str | None, Field(description="Resource service.name for this batch (default 'otel-streams-synth'). otel-logs keys a storage stream on (service.namespace, service.name); vary it per push to create distinct queryable services.")]
_ServiceNamespace = Annotated[str | None, Field(description="Resource service.namespace for this batch. Omitted emits no token (records reachable only via service.name; stream namespace defaults to '' for storage); pass '' explicitly to emit a queryable empty value. Pair with service_name to push multiple service streams for query testing.")]
_Tenant = Annotated[str | None, Field(description="Tenant id sent via the X-Scope-OrgID gRPC header.")]
_BatchSize = Annotated[int, Field(description="Max records per gRPC export request.", ge=1)]
_FlushMs = Annotated[int, Field(description="Max ms before flushing a partial batch.", ge=1)]
_ConnectTimeout = Annotated[int, Field(description="Max seconds to wait for the OTLP endpoint to accept a connection.", ge=1)]
_Timeout = Annotated[int, Field(description="Max seconds to wait for the whole push (the first run also builds synth).", ge=1, le=1200)]


class OtelPushResult(BaseModel):
    agent_id: str
    otel_endpoint: str | None = None
    count: int | None = Field(default=None, description="Records requested (and sent, when success=True).")
    success: bool = False
    returncode: int | None = None
    log_tail: str | None = Field(default=None, description="Last lines of the synth output (build + send).")
    error: str | None = None
    message: str = ""


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_push",
        description=(
            "Send a deterministic synthetic OTLP log corpus to a ready agent's otel "
            "plugin (one-shot; runs otel-streams `synth` to completion). Use with "
            "netdata_agent_otel_config's small rotation/retention thresholds + "
            "netdata_agent_otel_logs to verify storage edge cases over a known corpus. "
            "First call also builds synth (cargo), so allow time."
        ),
    )
    async def netdata_agent_otel_push(
        ctx: Context,
        agent_id: _AgentId,
        count: _Count = 100,
        field_cardinality: _FieldCard = 100,
        spacing_nanos: _Spacing = 1_000_000_000,
        start_time_nanos: _StartTime = None,
        seed: _Seed = 0,
        service_name: _ServiceName = None,
        service_namespace: _ServiceNamespace = None,
        tenant_id: _Tenant = None,
        batch_size: _BatchSize = 100,
        flush_interval_ms: _FlushMs = 1000,
        connect_timeout_secs: _ConnectTimeout = 30,
        timeout: _Timeout = 120,
    ) -> OtelPushResult:
        run = get_runs(ctx).get(agent_id)
        if run is None or run.state != "ready" or not run.otlp_endpoint:
            state = run.state if run is not None else "unknown"
            return OtelPushResult(
                agent_id=agent_id,
                error=f"Agent {agent_id!r} is not ready (state={state}) or has no OTLP endpoint. "
                "Start it with netdata_run_start and poll netdata_run_status until 'ready'.",
            )
        cmd = streams.synth_cmd(
            run.otlp_endpoint, count=count, field_cardinality=field_cardinality,
            spacing_nanos=spacing_nanos, start_time_nanos=start_time_nanos, seed=seed,
            tenant_id=tenant_id, batch_size=batch_size, flush_interval_ms=flush_interval_ms,
            connect_timeout_secs=connect_timeout_secs,
            service_name=service_name, service_namespace=service_namespace,
        )
        rc, tail, err = await streams.run_synth(run.worktree, cmd, timeout=timeout)
        if err is not None or rc != 0:
            return OtelPushResult(
                agent_id=agent_id, otel_endpoint=run.otlp_endpoint, count=count,
                success=False, returncode=rc, log_tail=tail,
                error=err or f"synth exited with code {rc}", message="push failed",
            )
        return OtelPushResult(
            agent_id=agent_id, otel_endpoint=run.otlp_endpoint, count=count,
            success=True, returncode=rc, log_tail=tail, message=f"sent {count} records",
        )
