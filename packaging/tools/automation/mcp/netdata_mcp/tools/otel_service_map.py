"""Otel service-map domain: a typed tool for the agent's ``topology:tracing``
Function (``otel-service-map/src/function.rs``).

The map is built from the stored traces over a time window and answered as a
``netdata.topology.v1`` payload (services as actors, calls as links). Errors are
the Functions ``{status, errorMessage}`` shape, including 413 for an answer over
the plugin's byte budget (narrow the window). Access is ``SIGNED_ID`` like
``otel-traces`` (see ``gated_function``).
"""

from __future__ import annotations

from typing import Annotated, Any

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .gated_function import AgentId, FunctionResult, Timeout, call_gated_function

_FUNCTION = "topology:tracing"

_Info = Annotated[bool, Field(description="If true, return the function's descriptor instead of a map.")]
_After = Annotated[int | None, Field(description="Window start, unix seconds; negative = relative to before. Omit for 15 minutes before 'before'.")]
_Before = Annotated[int | None, Field(description="Window end, unix seconds; negative = relative to now. Omit for now.")]
_Tenant = Annotated[str | None, Field(description="Tenant to read; omitted or empty reads the 'default' tenant.")]


def build_payload(*, info: bool, after: int | None, before: int | None, tenant: str | None) -> dict[str, Any]:
    """The request body, omitting unset fields so the function's defaults apply."""
    if info:
        return {"info": True}
    payload: dict[str, Any] = {}
    for key, value in (("after", after), ("before", before), ("tenant", tenant)):
        if value is not None:
            payload[key] = value
    return payload


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_service_map",
        description=(
            "Query a ready agent's service map (the topology:tracing Function): services "
            "and the calls between them, derived from stored traces over a window, as a "
            "netdata.topology.v1 payload. Window: after/before in unix seconds or negative "
            "relative seconds (default: the last 15 minutes; at most the plugin's maximum "
            "window). A 413 means the answer exceeded the plugin's byte budget: narrow the "
            "window. Feed it with netdata_agent_otel_push_traces. Access-gated (SIGNED_ID): "
            "needs NETDATA_CLOUD_TOKEN + a claimed agent, else returns HTTP 412."
        ),
    )
    async def netdata_agent_otel_service_map(
        ctx: Context,
        agent_id: AgentId,
        info: _Info = False,
        after: _After = None,
        before: _Before = None,
        tenant: _Tenant = None,
        timeout: Timeout = 60,
    ) -> FunctionResult:
        payload = build_payload(info=info, after=after, before=before, tenant=tenant)
        return await call_gated_function(ctx, agent_id, _FUNCTION, payload, timeout)
