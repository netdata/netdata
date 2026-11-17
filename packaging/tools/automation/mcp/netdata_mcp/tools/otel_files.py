"""Otel storage-files domain: a typed tool to list the storage files the
agent's otel-ledger worker is tracking.

POSTs ``{"files": true}`` to the agent's ``otel-logs`` function — a sibling mode
of the log query (``otel-ledger/src/ledger/rpc/logs/wire.rs``) — and returns the
per-tenant inventory of WAL / SFST (index) / catalog files, including the
``rotated`` / ``uploaded`` / ``remote_cataloged`` per-seq flags that have no
on-disk equivalent (a locally-evicted SFST can still be cataloged on the remote).

Access: the ``files`` mode shares ``otel-logs``'s ``SIGNED_ID`` gate (see
``gated_function``).
"""

from __future__ import annotations

from mcp.server.fastmcp import Context, FastMCP

from .gated_function import AgentId, FunctionResult, Timeout, call_gated_function

_FUNCTION = "otel-logs"


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_files",
        description=(
            "List the storage files the agent's otel-ledger is tracking — WAL, SFST "
            "(index), and catalog files — grouped by tenant, with per-file size, time "
            "range, record counts, and the rotated/uploaded/remote_cataloged/pending_deletion "
            "status that is NOT visible on disk (a locally-evicted SFST can still be cataloged "
            "on the remote). The response is {version, status, tenants:[{tenant, wal[], sfst[], "
            "catalog[]}]}. This is a storage-inventory snapshot; to read log CONTENT use "
            "netdata_agent_otel_logs. Pair with netdata_agent_otel_config's small "
            "rotation/retention thresholds + netdata_agent_otel_push_logs / "
            "netdata_agent_otel_push_traces to watch rotation, "
            "eviction, and remote upload happen. Access-gated (SIGNED_ID) like otel-logs: "
            "needs NETDATA_CLOUD_TOKEN + a claimed agent, else returns HTTP 412."
        ),
    )
    async def netdata_agent_otel_files(
        ctx: Context,
        agent_id: AgentId,
        timeout: Timeout = 60,
    ) -> FunctionResult:
        return await call_gated_function(ctx, agent_id, _FUNCTION, {"files": True}, timeout)
