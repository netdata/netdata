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

import yaml

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
# Per-signal tuning knobs come in symmetric logs_*/traces_* pairs (one call sets
# both signals; an omitted knob keeps that signal's stock default). Storage/auth
# are global (below), so they are NOT signal-prefixed.
_LogsRotationMaxFileSize = Annotated[str | None, Field(description="logs rotation: max size per data file (e.g. '25MB', '1.5GB'). Small values force rotation.")]
_LogsRotationMaxLogEntries = Annotated[int | None, Field(description="logs rotation: max log entries per data file. Tiny values (e.g. 10) force multi-file splits for edge-case tests.")]
_LogsRotationMaxFileDuration = Annotated[str | None, Field(description="logs rotation: max time span per data file (e.g. '2 hours', '30m').")]
_LogsCrcEnabled = Annotated[bool | None, Field(description="logs: verify stored data with checksums.")]
_LogsCompressionEnabled = Annotated[bool | None, Field(description="logs: compress stored data.")]
_LogsRetentionMaxFiles = Annotated[int | None, Field(description="logs retention: max number of index files to keep. Small values force eviction.")]
_LogsRetentionMaxTotalSize = Annotated[str | None, Field(description="logs retention: max total size of all index files (e.g. '1GB', '500MB').")]
_LogsCatalogRotationCount = Annotated[int | None, Field(description="logs catalog rotation: number of index files recorded before a catalog file rotates and uploads. Small values (e.g. 2) force catalog rotation + upload over a small corpus.")]
_TracesRotationMaxFileSize = Annotated[str | None, Field(description="traces rotation: max size per data file (e.g. '25MB', '1.5GB'). Small values force rotation.")]
_TracesRotationMaxLogEntries = Annotated[int | None, Field(description="traces rotation: max spans per data file. Tiny values (e.g. 10) force multi-file splits so a small trace corpus seals without a restart.")]
_TracesRotationMaxFileDuration = Annotated[str | None, Field(description="traces rotation: max time span per data file (e.g. '2 hours', '30m').")]
_TracesCrcEnabled = Annotated[bool | None, Field(description="traces: verify stored data with checksums.")]
_TracesCompressionEnabled = Annotated[bool | None, Field(description="traces: compress stored data.")]
_TracesRetentionMaxFiles = Annotated[int | None, Field(description="traces retention: max number of index files to keep. Small values force eviction.")]
_TracesRetentionMaxTotalSize = Annotated[str | None, Field(description="traces retention: max total size of all index files (e.g. '1GB', '500MB').")]
_TracesCatalogRotationCount = Annotated[int | None, Field(description="traces catalog rotation: number of index files recorded before a catalog file rotates and uploads. Small values (e.g. 2) force catalog rotation + upload over a small corpus.")]
_StorageEnabled = Annotated[bool | None, Field(description="Remote object-storage upload of SFST + catalog files (default off). Enable to exercise the upload path and remote-confirmed eviction. With it off, the uploader is not even constructed.")]
_StorageUri = Annotated[str | None, Field(description="opendal storage URI (e.g. 'fs:///abs/path', 's3://bucket/prefix'). Omit while storage is enabled to default to an isolated per-agent 'fs://' directory under the run dir.")]
_JournalDir = Annotated[
    str | None,
    Field(description="Read-only legacy viewer: directory of journal files written by the FORMER otel plugin, exposed via the 'legacy-otel-logs' function. The plugin only reads it (never writes/prunes). Unlike base_dir, it is NOT pinned under the run dir."),
]
_ExtraYaml = Annotated[
    str | None,
    Field(
        description=(
            "Raw-YAML escape hatch: a YAML MAPPING deep-merged over the generated "
            "otel.yaml (this passthrough wins on conflicts; nested mappings merge, "
            "other values replace). Reaches every knob without a first-class param — "
            "auth.enabled, logs.ingest.{max_age,future_skew}, "
            "logs.retention.default.{max_age,horizon}, logs.catalog.rotation_period, "
            "per-tenant rotation/retention override blocks, storage.startup_op_timeout — "
            "and deliberately-unknown keys for strict-config refuse-to-start tests. "
            "base_dir and endpoint.path stay pinned for per-agent isolation and "
            "cannot be overridden. SHARP TOOL: a semantically invalid config keeps "
            "the otel plugin down until reconfigured (check netdata_agent_logs "
            "component='supervisor'/'ledger' for the refusal) — that failure mode is "
            "itself the point of the refusal tests."
        )
    ),
]


def _extra_yaml_error(agent_id: str, value: str) -> RunInfo | None:
    """Return an error RunInfo if ``value`` is not parseable YAML with a mapping
    (or empty) at the top level, else None. Semantic validity is deliberately
    NOT checked — feeding the plugin a config it refuses is a supported test."""
    try:
        parsed = yaml.safe_load(value)
    except yaml.YAMLError as exc:
        return agent_error(agent_id, f"extra_yaml is not valid YAML: {exc}")
    if parsed is not None and not isinstance(parsed, dict):
        return agent_error(
            agent_id,
            f"extra_yaml must be a YAML mapping at the top level, got {type(parsed).__name__}",
        )
    return None


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_config",
        description=(
            "Set the otel-plugin configuration for a declared agent, applied on the "
            "next netdata_run_start (or restart=true). This REPLACES any prior otel "
            "config (it does not merge): each call, pass every knob you want set — an "
            "omitted knob reverts to the plugin default, not its prior value. The "
            "local wal/index/catalog dirs are always isolated under the agent's run "
            "dir. Tuning is PER SIGNAL: logs_* knobs tune the logs pipeline and "
            "traces_* knobs tune the traces pipeline independently (one call sets "
            "both). Storage is GLOBAL (not signal-prefixed; this tool exposes no "
            "auth knob): set storage_enabled=true (optionally storage_uri) to "
            "exercise the remote "
            "upload + remote-confirmed eviction path for both signals; an omitted "
            "storage_uri defaults to an isolated per-agent fs:// dir. Use the small "
            "rotation/retention knobs to force multi-file / eviction edge cases over a "
            "known corpus — set traces_* (e.g. traces_rotation_max_log_entries=10) so a "
            "small trace corpus seals without a restart. For knobs without a "
            "first-class param (auth, ingest windows, retention max_age/horizon, "
            "per-tenant overrides) or for strict-config refusal tests, pass a raw "
            "YAML mapping via extra_yaml — it deep-merges over the generated file "
            "and wins on conflicts (base_dir/endpoint.path stay pinned)."
        ),
    )
    async def netdata_agent_otel_config(
        ctx: Context,
        agent_id: _AgentId,
        otlp_endpoint: _Endpoint = None,
        logs_rotation_max_file_size: _LogsRotationMaxFileSize = None,
        logs_rotation_max_log_entries: _LogsRotationMaxLogEntries = None,
        logs_rotation_max_file_duration: _LogsRotationMaxFileDuration = None,
        logs_crc_enabled: _LogsCrcEnabled = None,
        logs_compression_enabled: _LogsCompressionEnabled = None,
        logs_retention_max_files: _LogsRetentionMaxFiles = None,
        logs_retention_max_total_size: _LogsRetentionMaxTotalSize = None,
        logs_catalog_rotation_count: _LogsCatalogRotationCount = None,
        traces_rotation_max_file_size: _TracesRotationMaxFileSize = None,
        traces_rotation_max_log_entries: _TracesRotationMaxLogEntries = None,
        traces_rotation_max_file_duration: _TracesRotationMaxFileDuration = None,
        traces_crc_enabled: _TracesCrcEnabled = None,
        traces_compression_enabled: _TracesCompressionEnabled = None,
        traces_retention_max_files: _TracesRetentionMaxFiles = None,
        traces_retention_max_total_size: _TracesRetentionMaxTotalSize = None,
        traces_catalog_rotation_count: _TracesCatalogRotationCount = None,
        storage_enabled: _StorageEnabled = None,
        storage_uri: _StorageUri = None,
        journal_dir: _JournalDir = None,
        extra_yaml: _ExtraYaml = None,
    ) -> RunInfo:
        if get_agents(ctx).get(agent_id) is None:
            return unknown_agent(agent_id)
        if otlp_endpoint is not None:
            err = _endpoint_error(agent_id, otlp_endpoint)
            if err is not None:
                return err
        if storage_uri is not None and "://" not in storage_uri:
            return agent_error(
                agent_id,
                f"storage_uri must be an opendal URI like 'fs:///path' or 's3://bucket', got {storage_uri!r}",
            )
        if extra_yaml is not None:
            err = _extra_yaml_error(agent_id, extra_yaml)
            if err is not None:
                return err
        cfg = OtelConfig(
            otlp_endpoint=otlp_endpoint,
            logs_rotation_max_file_size=logs_rotation_max_file_size,
            logs_rotation_max_log_entries=logs_rotation_max_log_entries,
            logs_rotation_max_file_duration=logs_rotation_max_file_duration,
            logs_crc_enabled=logs_crc_enabled,
            logs_compression_enabled=logs_compression_enabled,
            logs_retention_max_files=logs_retention_max_files,
            logs_retention_max_total_size=logs_retention_max_total_size,
            logs_catalog_rotation_count=logs_catalog_rotation_count,
            traces_rotation_max_file_size=traces_rotation_max_file_size,
            traces_rotation_max_log_entries=traces_rotation_max_log_entries,
            traces_rotation_max_file_duration=traces_rotation_max_file_duration,
            traces_crc_enabled=traces_crc_enabled,
            traces_compression_enabled=traces_compression_enabled,
            traces_retention_max_files=traces_retention_max_files,
            traces_retention_max_total_size=traces_retention_max_total_size,
            traces_catalog_rotation_count=traces_catalog_rotation_count,
            storage_enabled=storage_enabled,
            storage_uri=storage_uri,
            journal_dir=journal_dir,
            extra_yaml=extra_yaml,
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
