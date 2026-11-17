"""Otel-logs domain: a dedicated, typed tool to query a running agent's
``otel-logs`` function.

Unlike the generic ``netdata_agent_execute_function``, this POSTs an
``OtelLogsRequest`` straight to the agent's ``/api/v3/function?function=otel-logs``
endpoint with typed query params and returning the parsed response so an LLM
can assert on it (matched counts, facets, histogram, rows) — and probe the
function's capabilities with ``info=true``.

Access: ``otel-logs`` is ``SIGNED_ID``-gated; ``gated_function`` mints the bearer
(see its module doc for the 412 contract). The gate applies to ``info=true`` too.
"""

from __future__ import annotations

from typing import Annotated, Any, Literal

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .gated_function import AgentId, FunctionResult, Timeout, call_gated_function

_FUNCTION = "otel-logs"

_Info = Annotated[bool, Field(description="If true, return the function's capability descriptor (accepted params, help) instead of querying.")]
_After = Annotated[int | None, Field(description="Window start, unix seconds (inclusive). Omit for the function default.")]
_Before = Annotated[int | None, Field(description="Window end, unix seconds (exclusive).")]
_Query = Annotated[str | None, Field(description="Full-text query over key=value pairs (unanchored 'contains').")]
_Selections = Annotated[dict[str, list[str]] | None, Field(description="Per-field value filter: {field: [allowed values]} (OR within a field, AND across fields).")]
_Facets = Annotated[list[str] | None, Field(description="Fields to compute facet (value→count) breakdowns for.")]
_Histogram = Annotated[str | None, Field(description="Field to build the time histogram over.")]
_Direction = Annotated[Literal["forward", "backward"] | None, Field(description="Result order: newest-first (backward) or oldest-first (forward).")]
_Last = Annotated[int | None, Field(description="Max log entries to return.")]
_Anchor = Annotated[str | None, Field(description="Pagination anchor: an opaque row cursor from a prior response (a bare timestamp form also exists for histogram-bar clicks).")]
_Tenant = Annotated[str | None, Field(description="Tenant scoping selector; omitted reads the 'default' tenant.")]


def build_payload(
    *, info: bool, after: int | None, before: int | None, query: str | None,
    selections: dict[str, list[str]] | None, facets: list[str] | None, histogram: str | None,
    direction: str | None, last: int | None, anchor: str | None, tenant: str | None,
) -> dict[str, Any]:
    """Assemble the otel-logs request body, omitting unset fields so the
    function applies its own defaults (mirrors its `#[serde(default)]` shape)."""
    payload: dict[str, Any] = {}
    if info:
        payload["info"] = True
    for key, value in (
        ("after", after), ("before", before), ("query", query),
        ("histogram", histogram), ("direction", direction), ("last", last),
        ("anchor", anchor), ("tenant", tenant),
    ):
        if value is not None:
            payload[key] = value
    if selections:
        payload["selections"] = selections
    if facets:
        payload["facets"] = facets
    return payload


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_logs",
        description=(
            "Query a ready agent's otel-logs function with typed parameters, or probe it "
            "with info=true. POSTs directly to /api/v3/function?function=otel-logs and "
            "returns the parsed response (matched items, data rows, facets, histogram, "
            "pagination) for assertion. Pair with netdata_agent_otel_config's small "
            "rotation/retention thresholds + pushed test logs to verify storage edge cases."
        ),
    )
    async def netdata_agent_otel_logs(
        ctx: Context,
        agent_id: AgentId,
        info: _Info = False,
        after: _After = None,
        before: _Before = None,
        query: _Query = None,
        selections: _Selections = None,
        facets: _Facets = None,
        histogram: _Histogram = None,
        direction: _Direction = None,
        last: _Last = None,
        anchor: _Anchor = None,
        tenant: _Tenant = None,
        timeout: Timeout = 60,
    ) -> FunctionResult:
        payload = build_payload(
            info=info, after=after, before=before, query=query, selections=selections,
            facets=facets, histogram=histogram, direction=direction, last=last,
            anchor=anchor, tenant=tenant,
        )
        return await call_gated_function(ctx, agent_id, _FUNCTION, payload, timeout)
