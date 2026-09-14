"""Otel storage-files domain: a typed tool to list the storage files the
agent's otel-ledger worker is tracking.

POSTs ``{"files": true}`` to the agent's ``otel-logs`` function — a sibling mode
of the log query (``otel-ledger/src/ledger/rpc/handler.rs``) — and returns the
per-tenant inventory of WAL / SFST (index) / catalog files, including the
``rotated`` / ``uploaded`` / ``remote_cataloged`` per-seq flags that have no
on-disk equivalent (a locally-evicted SFST can still be cataloged on the remote).

Access: the ``files`` mode shares ``otel-logs``'s gate (``SIGNED_ID | SAME_SPACE
| SENSITIVE_DATA``), so this mints+sends a Cloud bearer when ``NETDATA_CLOUD_TOKEN``
is set (claimed agent), else the call returns HTTP 412 — same contract as
``netdata_agent_otel_logs`` (see ``bearer.py`` / ``otel_logs.py``).
"""

from __future__ import annotations

from typing import Annotated, Any

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, Field

from .. import agentfn, bearer
from ._common import get_runs
from .otel_logs import classify_status

_FUNCTION = "otel-logs"

# Cap for the bearer mint's HTTP hops, kept below the call timeout so it can't
# starve the function call (mirrors otel_logs).
_MINT_TIMEOUT = 20

_AgentId = Annotated[str, Field(description="A ready agent (from netdata_agent_declare + netdata_run_start).")]
_Timeout = Annotated[int, Field(description="Seconds to wait for the function.", ge=1, le=600)]


class OtelFilesResult(BaseModel):
    """Result of an otel-logs files-inventory call. ``response`` is the parsed
    function output: ``{version, status, tenants:[{tenant, wal[], sfst[],
    catalog[]}]}`` — each file carries size, time range, and (for SFST) the
    rotated/uploaded/remote_cataloged/pending_deletion flags — so callers can
    assert on any field."""

    agent_id: str
    endpoint: str | None = None
    http_status: int | None = Field(default=None, description="HTTP status from the function call.")
    response: dict[str, Any] | None = Field(default=None, description="Parsed inventory (version/status/tenants).")
    error: str | None = None
    message: str = ""


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_files",
        description=(
            "List the storage files the agent's otel-ledger is tracking — WAL, SFST "
            "(index), and catalog files — grouped by tenant, with per-file size, time "
            "range, record counts, and the rotated/uploaded/remote_cataloged/pending_deletion "
            "status that is NOT visible on disk (a locally-evicted SFST can still be cataloged "
            "on the remote). This is a storage-inventory snapshot; to read log CONTENT use "
            "netdata_agent_otel_logs. Pair with netdata_agent_otel_config's small "
            "rotation/retention thresholds + netdata_agent_otel_push_logs / "
            "netdata_agent_otel_push_traces to watch rotation, "
            "eviction, and remote upload happen. Access-gated (SIGNED_ID) like otel-logs: "
            "needs NETDATA_CLOUD_TOKEN + a claimed agent, else returns HTTP 412."
        ),
    )
    async def netdata_agent_otel_files(
        ctx: Context,
        agent_id: _AgentId,
        timeout: _Timeout = 60,
    ) -> OtelFilesResult:
        run = get_runs(ctx).get(agent_id)
        if run is None or run.state != "ready" or not run.port:
            state = run.state if run is not None else "unknown"
            return OtelFilesResult(
                agent_id=agent_id,
                error=f"Agent {agent_id!r} is not ready (state={state}). "
                "Start it with netdata_run_start and poll netdata_run_status until 'ready'.",
            )
        payload: dict[str, Any] = {"files": True}
        base = f"http://127.0.0.1:{run.port}"
        endpoint = agentfn.function_url(base, _FUNCTION, timeout)
        # Same SIGNED_ID gate as otel-logs: mint a bearer when a Cloud token is
        # present (a mint failure is hard — an anonymous retry would just 412);
        # without a token, call anonymously and let the 412 hint explain.
        auth: str | None = None
        if bearer.cloud_token():
            auth, berr = await bearer.resolve_bearer(run.port, timeout=min(timeout, _MINT_TIMEOUT))
            if auth is None:
                return OtelFilesResult(
                    agent_id=agent_id, endpoint=endpoint,
                    error=f"could not obtain a Netdata Cloud bearer ({berr}). "
                    "The files inventory requires a signed-in identity; ensure the agent "
                    "is claimed and cloud_connected (netdata_run_status).",
                )
        status, data, err = await agentfn.call_function(
            base, _FUNCTION, payload, timeout=timeout, bearer=auth
        )
        if err is not None:
            return OtelFilesResult(agent_id=agent_id, endpoint=endpoint, http_status=status, error=err)
        result_error, message = classify_status(status)
        return OtelFilesResult(
            agent_id=agent_id, endpoint=endpoint, http_status=status,
            response=data if isinstance(data, dict) else {"value": data},
            error=result_error, message=message,
        )
