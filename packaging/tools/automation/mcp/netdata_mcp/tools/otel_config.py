"""Otel domain: tune the otel-plugin config applied to an agent at launch.

A declared agent carries an :class:`~netdata_mcp.runtime.OtelConfig`; this tool
sets it. The config is written into the agent's ``otel.yaml`` on the next
``netdata_run_start`` / restart — so the edit-a-knob → restart → re-query loop
is how an LLM forces storage edge cases (rotation, retention) over small,
deterministic corpora.
"""

from __future__ import annotations

import re
from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from ..runtime import OtelConfig
from ._common import get_agents, get_runs
from .models import RunInfo, agent_declared, agent_error, run_info, unknown_agent

# Reject a malformed OTLP endpoint here so the caller gets a clean error now,
# instead of an opaque agent-launch failure several tool calls later.
_HOST_PORT_RE = re.compile(r"^[^\s:]+:(\d{1,5})$")


def _endpoint_error(agent_id: str, value: str) -> RunInfo | None:
    """Return an error RunInfo if ``value`` is not a valid host:port, else None."""
    m = _HOST_PORT_RE.match(value)
    if m is None:
        return agent_error(agent_id, f"otlp_endpoint must be 'host:port', got {value!r}")
    if not (1 <= int(m.group(1)) <= 65535):
        return agent_error(agent_id, f"otlp_endpoint port out of range (1-65535): {value!r}")
    return None

_AgentId = Annotated[str, Field(description="The declared agent to configure.")]
_Endpoint = Annotated[
    str | None,
    Field(description="OTLP/gRPC listen address 'host:port' (e.g. '127.0.0.1:4317'). Omit to auto-assign a free loopback port."),
]
_MaxFileSize = Annotated[str | None, Field(description="WAL rotation: max size per WAL file (e.g. '25MB', '1.5GB'). Small values force rotation.")]
_MaxLogEntries = Annotated[int | None, Field(description="WAL rotation: max log entries per WAL file. Tiny values (e.g. 10) force multi-file splits for edge-case tests.")]
_MaxFileDuration = Annotated[str | None, Field(description="WAL rotation: max time span per WAL file (e.g. '2 hours', '30m').")]
_CrcEnabled = Annotated[bool | None, Field(description="WAL: compute per-frame CRC32 checksums.")]
_CompressionEnabled = Annotated[bool | None, Field(description="WAL: LZ4-compress frame payloads.")]
_MaxFiles = Annotated[int | None, Field(description="Index retention: max number of SFST index files to keep. Small values force eviction.")]
_MaxTotalSize = Annotated[str | None, Field(description="Index retention: max total size of all index files (e.g. '1GB', '500MB').")]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_config",
        description=(
            "Set the otel-plugin configuration for a declared agent, applied on the "
            "next netdata_run_start (or restart=true). This REPLACES any prior otel "
            "config (it does not merge): each call, pass every knob you want set — an "
            "omitted knob reverts to the plugin default, not its prior value. Storage "
            "dirs are always isolated under the agent's run dir. Use the rotation/"
            "retention knobs with small values to force multi-file / eviction edge "
            "cases over a known corpus."
        ),
    )
    async def netdata_agent_otel_config(
        ctx: Context,
        agent_id: _AgentId,
        otlp_endpoint: _Endpoint = None,
        wal_max_file_size: _MaxFileSize = None,
        wal_max_log_entries: _MaxLogEntries = None,
        wal_max_file_duration: _MaxFileDuration = None,
        wal_crc_enabled: _CrcEnabled = None,
        wal_compression_enabled: _CompressionEnabled = None,
        index_max_files: _MaxFiles = None,
        index_max_total_size: _MaxTotalSize = None,
    ) -> RunInfo:
        if get_agents(ctx).get(agent_id) is None:
            return unknown_agent(agent_id)
        if otlp_endpoint is not None:
            err = _endpoint_error(agent_id, otlp_endpoint)
            if err is not None:
                return err
        cfg = OtelConfig(
            otlp_endpoint=otlp_endpoint,
            wal_max_file_size=wal_max_file_size,
            wal_max_log_entries=wal_max_log_entries,
            wal_max_file_duration=wal_max_file_duration,
            wal_crc_enabled=wal_crc_enabled,
            wal_compression_enabled=wal_compression_enabled,
            index_max_files=index_max_files,
            index_max_total_size=index_max_total_size,
        )
        spec = get_agents(ctx).set_otel(agent_id, cfg)
        live = get_runs(ctx).get(agent_id)
        if live is not None and not live.done:
            return run_info(
                live,
                message=f"otel config set for {agent_id!r}. Restart the agent "
                "(netdata_run_start restart=true) to apply.",
            )
        return agent_declared(
            spec, message=f"otel config set for {agent_id!r}. Applies on the next netdata_run_start."
        )
