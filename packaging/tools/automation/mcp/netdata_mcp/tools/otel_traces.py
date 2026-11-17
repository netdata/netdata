"""Otel-traces domain: a typed tool for the agent's ``otel-traces`` Function
(``otel-ledger/src/ledger/rpc/traces/wire.rs``).

The Function has a selector-less list view plus four modes, each selected by
one top-level object (``info``, ``trace``, ``suggest``, ``explore``) holding
that mode's params. This tool takes one flat set of typed params plus ``mode``
and builds the matching wire body; a param that the chosen mode does not read
is refused here instead of being dropped. Values are passed through unchecked,
so the Function's own validation stays observable. Access is ``SIGNED_ID``
(see ``gated_function``).
"""

from __future__ import annotations

from typing import Annotated, Any, Literal

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, ConfigDict, Field

from .gated_function import AgentId, FunctionResult, Timeout, call_gated_function

_FUNCTION = "otel-traces"

Mode = Literal["list", "info", "trace", "suggest", "explore"]


class Condition(BaseModel):
    """A search-bar chip."""

    model_config = ConfigDict(extra="forbid")

    key: str = Field(description=(
        "Field key, e.g. 'resource.attributes.service.name', 'name', 'duration', 'status.code'."
    ))
    op: str = Field(description=(
        "One of = != =~ !~ > >= < <= ^= !^= ; '!' ops mean NOT EXISTS, '=~' is a whole-value regex, '^=' is "
        "starts-with."
    ))
    values: list[str | int | float] = Field(description=(
        "Values OR'd together (ordering ops take one). Durations are integer nanoseconds."
    ))


# Which params each mode reads; everything else set for that mode is refused.
_MODE_PARAMS: dict[str, frozenset[str]] = {
    "list": frozenset({"after", "before", "last", "anchor", "conditions", "text", "skip_overview"}),
    "info": frozenset(),
    "trace": frozenset({"trace_id", "after", "before", "conditions", "text"}),
    "suggest": frozenset({"after", "before", "key", "term", "max_items", "tier"}),
    "explore": frozenset({
        "after", "before", "conditions", "text", "anchor",
        "histogram", "histogram_stack", "rows", "rows_limit", "rows_columns",
    }),
}

_Mode = Annotated[Mode, Field(description=(
    "list: the trace list (the default Functions view; first page also carries the window overview). "
    "info: capability descriptor. trace: one trace's spans by trace_id. suggest: search-bar key "
    "suggestions, or one key's values with 'key'. explore: the span view (histogram and/or rows of spans)."
))]
_Tenant = Annotated[str | None, Field(description="Tenant to read (all modes but info); omitted reads 'default'.")]
_After = Annotated[int | None, Field(description=(
    "Window start, unix seconds. list/explore: negative = relative to before, 0/omitted = 15 min before 'before'. "
    "suggest: absolute only. trace: optional assembly bound, set together with before."
))]
_Before = Annotated[int | None, Field(description=(
    "Window end, unix seconds. list/explore: negative = relative to now, 0/omitted = now. "
    "suggest: absolute only. trace: optional assembly bound, set together with after."
))]
_Conditions = Annotated[list[Condition] | None, Field(description=(
    "Search-bar chips (list/trace/explore). list: AND at trace level. explore: AND per span "
    "(trace-level keys refused; trace_id selects spans of given traces). trace: marks matching spans."
))]
_Text = Annotated[list[str] | None, Field(description=(
    "Free-text tokens, each at least 3 chars, case-insensitive 'contains' over span name, status message, event names"
    " and attribute values."
))]
_Anchor = Annotated[str | None, Field(description=(
    "Pagination cursor echoed from a previous answer (list: anchor.next; explore: rows.anchor.next)."
))]
_Last = Annotated[int | None, Field(description="list: traces per page (1-200).")]
_SkipOverview = Annotated[bool | None, Field(description="list: true skips the window overview section.")]
_TraceId = Annotated[str | None, Field(description="trace: the trace id, 32 hex chars.")]
_Key = Annotated[str | None, Field(description="suggest: the key whose values to suggest; omit to suggest keys.")]
_Term = Annotated[str | None, Field(description="suggest: typed prefix text; empty matches everything.")]
_MaxItems = Annotated[int | None, Field(description="suggest: cap on suggestions (1-200); not with tier.")]
_Tier = Annotated[Literal["low", "mid", "high"] | None, Field(description=(
    "suggest: every key of this tier (keys only, not with key/max_items)."
))]
_Histogram = Annotated[bool | None, Field(description=(
    "explore: ask for the histogram section (implied by histogram_stack)."
))]
_HistogramStack = Annotated[str | None, Field(description=(
    "explore: key whose values stack the histogram (default status.code)."
))]
_Rows = Annotated[bool | None, Field(description=(
    "explore: ask for a page of spans, newest first (implied by rows_limit/rows_columns)."
))]
_RowsLimit = Annotated[int | None, Field(description="explore: spans per page.")]
_RowsColumns = Annotated[list[str] | None, Field(description=(
    "explore: keys each span row carries (default service, name, kind, status)."
))]


def build_payload(mode: str, tenant: str | None, params: dict[str, Any]) -> dict[str, Any]:
    """Build the wire body for ``mode`` from the set (non-None) ``params``.
    Raises ``ValueError`` naming every param the mode does not read."""
    set_params = {k: v for k, v in params.items() if v is not None}
    stray = sorted(set(set_params) - _MODE_PARAMS[mode])
    if tenant is not None and mode == "info":
        stray.append("tenant")
    if stray:
        raise ValueError(f"mode {mode!r} does not take: {', '.join(stray)}")
    if "conditions" in set_params:
        set_params["conditions"] = [
            c.model_dump() if isinstance(c, BaseModel) else c for c in set_params["conditions"]
        ]

    body: dict[str, Any]
    if mode == "list":
        body = set_params
    elif mode == "info":
        body = {"info": {}}
    elif mode == "trace":
        sel = {("id" if k == "trace_id" else k): v for k, v in set_params.items()}
        body = {"trace": sel}
    elif mode == "suggest":
        body = {"suggest": set_params}
    else:
        sel = {k: set_params[k] for k in ("after", "before", "conditions", "text", "anchor") if k in set_params}
        if set_params.get("histogram") or "histogram_stack" in set_params:
            hist: dict[str, Any] = {}
            if "histogram_stack" in set_params:
                hist["stack"] = set_params["histogram_stack"]
            sel["histogram"] = hist
        if set_params.get("rows") or "rows_limit" in set_params or "rows_columns" in set_params:
            rows: dict[str, Any] = {}
            if "rows_limit" in set_params:
                rows["limit"] = set_params["rows_limit"]
            if "rows_columns" in set_params:
                rows["columns"] = set_params["rows_columns"]
            sel["rows"] = rows
        body = {"explore": sel}
    if tenant is not None:
        body["tenant"] = tenant
    return body


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_traces",
        description=(
            "Query a ready agent's otel-traces Function. Pick a mode: 'list' (trace list + window "
            "overview; page with anchor), 'info', 'trace' (spans of one trace_id), 'suggest' "
            "(search-bar keys, or a key's values), 'explore' (span view: histogram and/or rows). "
            "Each param says which modes read it; a param the mode does not read is refused. "
            "Returns the parsed response for assertion. Feed it with netdata_agent_otel_push_traces "
            "(use a distinct seed per push, or trace/span ids repeat across runs). Access-gated "
            "(SIGNED_ID): needs NETDATA_CLOUD_TOKEN + a claimed agent, else returns HTTP 412."
        ),
    )
    async def netdata_agent_otel_traces(
        ctx: Context,
        agent_id: AgentId,
        mode: _Mode = "list",
        tenant: _Tenant = None,
        after: _After = None,
        before: _Before = None,
        conditions: _Conditions = None,
        text: _Text = None,
        anchor: _Anchor = None,
        last: _Last = None,
        skip_overview: _SkipOverview = None,
        trace_id: _TraceId = None,
        key: _Key = None,
        term: _Term = None,
        max_items: _MaxItems = None,
        tier: _Tier = None,
        histogram: _Histogram = None,
        histogram_stack: _HistogramStack = None,
        rows: _Rows = None,
        rows_limit: _RowsLimit = None,
        rows_columns: _RowsColumns = None,
        timeout: Timeout = 60,
    ) -> FunctionResult:
        params = dict(
            after=after, before=before, conditions=conditions, text=text, anchor=anchor,
            last=last, skip_overview=skip_overview, trace_id=trace_id, key=key, term=term,
            max_items=max_items, tier=tier, histogram=histogram, histogram_stack=histogram_stack,
            rows=rows, rows_limit=rows_limit, rows_columns=rows_columns,
        )
        try:
            payload = build_payload(mode, tenant, params)
        except ValueError as exc:
            return FunctionResult(agent_id=agent_id, function=_FUNCTION, error=str(exc))
        return await call_gated_function(ctx, agent_id, _FUNCTION, payload, timeout)
