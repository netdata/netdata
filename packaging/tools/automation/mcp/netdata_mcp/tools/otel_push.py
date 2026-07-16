"""Otel push: send a deterministic synthetic OTLP corpus to a ready agent.

One tool per signal (`netdata_agent_otel_push_logs` / `..._traces`), each one-shot
and synchronous: it runs the matching `otel-streams` generator (`synth` /
`synth-traces`) to completion against the agent's OTLP endpoint and returns the
outcome. Pair with `netdata_agent_otel_config` (tiny per-signal rotation
thresholds) + `netdata_agent_otel_files` to verify storage edge cases over a
known-exact corpus. For continuous real-world input use the
`netdata_agent_otel_stream_*` tools instead.
"""

from __future__ import annotations

from collections.abc import Callable
from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, Field

from .. import streams
from ._common import get_runs

_AgentId = Annotated[str, Field(description="A ready agent (declare + run_start) to push to.")]
_Count = Annotated[int, Field(description="Number of synthetic records to send.", ge=1, le=10_000_000)]
_FieldCard = Annotated[int, Field(description="Distinct values per mid-cardinality attribute (host, code).", ge=1)]
_Spacing = Annotated[int, Field(description="Nanoseconds between consecutive records.", ge=0)]
_Duration = Annotated[int, Field(description="Per-span duration in nanoseconds (span end = start + this).", ge=0)]
_StartTime = Annotated[int | None, Field(description="First record timestamp (unix nanos). Default: now − count·spacing, so the batch lands in the recent past.", ge=0)]
_Seed = Annotated[int, Field(description="Deterministic value-selection offset (for logs, seeds within [0, field_cardinality) give distinct corpora; for traces it offsets span id/value derivation).", ge=0)]
_ServiceName = Annotated[str | None, Field(description="Resource service.name for this batch. The storage stream is keyed on (service.namespace, service.name); vary it per push to create distinct queryable services.")]
_ServiceNamespace = Annotated[str | None, Field(description="Resource service.namespace for this batch. Omitted emits no token (records reachable only via service.name; stream namespace defaults to '' for storage); pass '' explicitly to emit a queryable empty value.")]
_Tenant = Annotated[str | None, Field(description="Tenant id sent via the X-Scope-OrgID gRPC header.")]
_BatchSize = Annotated[int, Field(description="Max records per gRPC export request.", ge=1)]
_FlushMs = Annotated[int, Field(description="Max ms before flushing a partial batch.", ge=1)]
_ConnectTimeout = Annotated[int, Field(description="Max seconds to wait for the OTLP endpoint to accept a connection.", ge=1)]
_Timeout = Annotated[int, Field(description="Max seconds to wait for the whole push (the first run also builds the generator).", ge=1, le=1200)]


class OtelPushResult(BaseModel):
    agent_id: str
    otel_endpoint: str | None = None
    count: int | None = Field(default=None, description="Records requested (and sent, when success=True).")
    success: bool = False
    returncode: int | None = None
    log_tail: str | None = Field(default=None, description="Last lines of the generator output (build + send).")
    error: str | None = None
    message: str = ""


async def _run_push(
    ctx: Context, agent_id: str, count: int,
    cmd_builder: Callable[[str], list[str]], timeout: int,
) -> OtelPushResult:
    """Shared push path: validate the agent is ready, build the argv with the
    signal-specific ``cmd_builder(otel_endpoint)``, run the generator, map the
    outcome. Both push tools funnel through here so the ready-check, run, and
    result-shaping stay identical across signals.
    """
    run = get_runs(ctx).get(agent_id)
    if run is None or run.state != "ready" or not run.otlp_endpoint:
        state = run.state if run is not None else "unknown"
        return OtelPushResult(
            agent_id=agent_id, message="agent not ready",
            error=f"Agent {agent_id!r} is not ready (state={state}) or has no OTLP endpoint. "
            "Start it with netdata_run_start and poll netdata_run_status until 'ready'.",
        )
    cmd = cmd_builder(run.otlp_endpoint)
    rc, tail, err = await streams.run_synth(run.worktree, cmd, timeout=timeout)
    if err is not None or rc != 0:
        return OtelPushResult(
            agent_id=agent_id, otel_endpoint=run.otlp_endpoint, count=count,
            success=False, returncode=rc, log_tail=tail,
            error=err or f"generator exited with code {rc}", message="push failed",
        )
    return OtelPushResult(
        agent_id=agent_id, otel_endpoint=run.otlp_endpoint, count=count,
        success=True, returncode=rc, log_tail=tail, message=f"sent {count} records",
    )


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_push_logs",
        description=(
            "Send a deterministic synthetic OTLP LOG corpus to a ready agent's otel "
            "plugin (one-shot; runs otel-streams `synth` to completion). Use with "
            "netdata_agent_otel_config's small logs_* rotation/retention thresholds + "
            "netdata_agent_otel_files to verify storage edge cases over a known corpus. "
            "For traces use netdata_agent_otel_push_traces. First call also builds the "
            "generator (cargo), so allow time."
        ),
    )
    async def netdata_agent_otel_push_logs(
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
        return await _run_push(
            ctx, agent_id, count,
            lambda ep: streams.synth_logs_cmd(
                ep, count=count, field_cardinality=field_cardinality,
                spacing_nanos=spacing_nanos, start_time_nanos=start_time_nanos, seed=seed,
                tenant_id=tenant_id, batch_size=batch_size, flush_interval_ms=flush_interval_ms,
                connect_timeout_secs=connect_timeout_secs,
                service_name=service_name, service_namespace=service_namespace,
            ),
            timeout,
        )

    @mcp.tool(
        name="netdata_agent_otel_push_traces",
        description=(
            "Send a deterministic synthetic OTLP TRACE corpus (spans) to a ready agent's "
            "otel plugin (one-shot; runs otel-streams `synth-traces` to completion). Use "
            "with netdata_agent_otel_config's small traces_* rotation thresholds (e.g. "
            "traces_rotation_max_entries=10) + netdata_agent_otel_files to watch the traces "
            "pipeline rotate + seal + upload over a known corpus WITHOUT an additional "
            "restart (the small thresholds, applied at the prior run_start, make rotation "
            "automatic as data arrives). Like "
            "the logs push but with `duration_nanos` (per-span duration) and no "
            "`field_cardinality`. First call also builds the generator (cargo), so allow time."
        ),
    )
    async def netdata_agent_otel_push_traces(
        ctx: Context,
        agent_id: _AgentId,
        count: _Count = 100,
        spacing_nanos: _Spacing = 1_000_000_000,
        duration_nanos: _Duration = 5_000_000,
        start_time_nanos: _StartTime = None,
        seed: _Seed = 0,
        service_name: _ServiceName = None,
        service_namespace: _ServiceNamespace = None,
        tenant_id: _Tenant = None,
        batch_size: _BatchSize = 100,
        connect_timeout_secs: _ConnectTimeout = 30,
        timeout: _Timeout = 120,
    ) -> OtelPushResult:
        # No flush_interval_ms: synth-traces exports synchronously in batch_size
        # chunks (no flush timer), so the logs Sender's flush knob is inert here.
        return await _run_push(
            ctx, agent_id, count,
            lambda ep: streams.synth_traces_cmd(
                ep, count=count, spacing_nanos=spacing_nanos, duration_nanos=duration_nanos,
                start_time_nanos=start_time_nanos, seed=seed,
                tenant_id=tenant_id, batch_size=batch_size,
                connect_timeout_secs=connect_timeout_secs,
                service_name=service_name, service_namespace=service_namespace,
            ),
            timeout,
        )
