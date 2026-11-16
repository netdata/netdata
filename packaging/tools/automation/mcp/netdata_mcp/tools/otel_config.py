"""Otel domain: tune the otel-plugin config applied to an agent at launch.

A declared agent carries an :class:`~netdata_mcp.runtime.OtelConfig`; this tool
sets it. The config is written into the agent's ``otel.yaml`` on the next
``netdata_run_start`` / restart — so the edit-a-knob → restart → re-query loop
is how an LLM forces storage edge cases (rotation, retention) over small,
deterministic corpora.
"""

from __future__ import annotations

import ipaddress
import re
from typing import Annotated

import yaml

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from ..runtime import OtelConfig, web_port_clash
from ._common import get_agents, get_runs
from .models import RunInfo, agent_declared, agent_error, run_info, unknown_agent

# Reject a malformed OTLP endpoint here so the caller gets a clean error now,
# instead of an opaque agent-launch failure several tool calls later. The
# plugin parses both listeners as a Rust SocketAddr: an IP literal (IPv6 in
# brackets) and a port, never a hostname.
_HOST_PORT_RE = re.compile(r"(?:\[([0-9A-Fa-f:.]+)\]|([0-9.]+)):([0-9]{1,5})", re.ASCII)


def _endpoint_error(agent_id: str, value: str, name: str = "otlp_endpoint") -> RunInfo | None:
    """Return an error RunInfo if ``value`` is not a valid ip:port, else None."""
    m = _HOST_PORT_RE.fullmatch(value)
    if m is None or not _is_ip(m.group(1), 6) and not _is_ip(m.group(2), 4):
        return agent_error(
            agent_id,
            f"{name} must be 'ip:port' with an IP address, not a hostname "
            f"(e.g. '127.0.0.1:4317', '[::1]:4317'), got {value!r}",
        )
    if not (1 <= int(m.group(3)) <= 65535):
        return agent_error(agent_id, f"{name} port out of range (1-65535): {value!r}")
    return None


def _is_ip(text: str | None, version: int) -> bool:
    try:
        return text is not None and ipaddress.ip_address(text).version == version
    except ValueError:
        return False


_AgentId = Annotated[str, Field(description="The declared agent to configure.")]
_Endpoint = Annotated[
    str | None,
    Field(
        description=(
            "OTLP/gRPC listen address 'ip:port' (e.g. '127.0.0.1:4317'). Omit to auto-assign "
            "a free loopback port. Refused on the agent's declared web port (127.0.0.1 or a "
            "wildcard address)."
        )
    ),
]
_HttpEndpoint = Annotated[
    str | None,
    Field(
        description=(
            "OTLP/HTTP listen address 'ip:port' (e.g. '127.0.0.1:4318'). Omit to "
            "auto-assign a free loopback port; pass the EMPTY STRING '' to disable "
            "the HTTP listener (writes its enabled: false); any other value must be "
            "'ip:port', and is refused on the agent's declared web port (127.0.0.1 "
            "or a wildcard address)."
        )
    ),
]
# Per-signal tuning knobs come in symmetric logs_*/traces_* pairs (one call sets
# both signals; an omitted knob keeps that signal's stock default). Storage/auth
# are global (below), so they are NOT signal-prefixed.
_LogsRotationMaxFileSize = Annotated[str | None, Field(description="logs rotation: max size per data file (e.g. '25MB', '1.5GB'). Small values force rotation.")]
_LogsRotationMaxEntries = Annotated[int | None, Field(description="logs rotation: max entries per data file. Tiny values (e.g. 10) force multi-file splits for edge-case tests.")]
_LogsRotationMaxFileDuration = Annotated[str | None, Field(description="logs rotation: max time span per data file (e.g. '2 hours', '30m').")]
_LogsCrcEnabled = Annotated[bool | None, Field(description="logs: verify stored data with checksums.")]
_LogsCompressionEnabled = Annotated[bool | None, Field(description="logs: compress stored data.")]
_LogsRetentionMaxFiles = Annotated[int | None, Field(description="logs retention: max number of index files to keep. Small values force eviction.")]
_LogsRetentionMaxTotalSize = Annotated[str | None, Field(description="logs retention: max total size of all index files (e.g. '1GB', '500MB').")]
_LogsCatalogRotationCount = Annotated[int | None, Field(description="logs catalog rotation: number of index files recorded before a catalog file rotates and uploads. Small values (e.g. 2) force catalog rotation + upload over a small corpus.")]
_TracesRotationMaxFileSize = Annotated[str | None, Field(description="traces rotation: max size per data file (e.g. '25MB', '1.5GB'). Small values force rotation.")]
_TracesRotationMaxEntries = Annotated[int | None, Field(description="traces rotation: max spans per data file. Tiny values (e.g. 10) force multi-file splits so a small trace corpus seals without a restart.")]
_TracesRotationMaxFileDuration = Annotated[str | None, Field(description="traces rotation: max time span per data file (e.g. '2 hours', '30m').")]
_TracesCrcEnabled = Annotated[bool | None, Field(description="traces: verify stored data with checksums.")]
_TracesCompressionEnabled = Annotated[bool | None, Field(description="traces: compress stored data.")]
_TracesRetentionMaxFiles = Annotated[int | None, Field(description="traces retention: max number of index files to keep. Small values force eviction.")]
_TracesRetentionMaxTotalSize = Annotated[str | None, Field(description="traces retention: max total size of all index files (e.g. '1GB', '500MB').")]
_TracesCatalogRotationCount = Annotated[int | None, Field(description="traces catalog rotation: number of index files recorded before a catalog file rotates and uploads. Small values (e.g. 2) force catalog rotation + upload over a small corpus.")]
_RemoteStorageEnabled = Annotated[bool | None, Field(description="Remote object-storage upload of SFST + catalog files (default off). Enable to exercise the upload path and remote-confirmed eviction. With it off, the uploader is not even constructed.")]
_RemoteStorageUri = Annotated[str | None, Field(description="opendal storage URI (e.g. 'fs:///abs/path', 's3://bucket/prefix'). Omit while remote storage is enabled to default to an isolated per-agent 'fs://' directory under the run dir.")]
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
            "per-tenant rotation/retention override blocks, remote_storage.startup_op_timeout, "
            "remote_storage.read_cache_max_size (the download cache both signals share) — "
            "and deliberately-unknown keys for strict-config refuse-to-start tests. "
            "base_dir, both listener endpoints, and the HTTP listener's enabled flag stay pinned for "
            "per-agent isolation and cannot be overridden. SHARP TOOL: a semantically invalid config keeps "
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
            "both). Storage is GLOBAL (not signal-prefixed; auth has no "
            "first-class param — reach it via extra_yaml): set "
            "remote_storage_enabled=true (optionally remote_storage_uri) to exercise the remote "
            "upload + remote-confirmed eviction path for both signals; an omitted "
            "remote_storage_uri defaults to an isolated per-agent fs:// dir. Use the small "
            "rotation/retention knobs to force multi-file / eviction edge cases over a "
            "known corpus — set traces_* (e.g. traces_rotation_max_entries=10) so a "
            "small trace corpus seals without a restart. otlp_http_endpoint sets the "
            "OTLP/HTTP listener (receivers.otlp.protocols.http): omit to auto-assign a free "
            "loopback port, '' to disable it, 'ip:port' to pin it. For knobs without a "
            "first-class param (auth, ingest windows, retention max_age/horizon, "
            "per-tenant overrides) or for strict-config refusal tests, pass a raw "
            "YAML mapping via extra_yaml — it deep-merges over the generated file "
            "and wins on conflicts (base_dir, both listener endpoints, and the HTTP "
            "listener's enabled flag stay pinned)."
        ),
    )
    async def netdata_agent_otel_config(
        ctx: Context,
        agent_id: _AgentId,
        otlp_endpoint: _Endpoint = None,
        otlp_http_endpoint: _HttpEndpoint = None,
        logs_rotation_max_file_size: _LogsRotationMaxFileSize = None,
        logs_rotation_max_entries: _LogsRotationMaxEntries = None,
        logs_rotation_max_file_duration: _LogsRotationMaxFileDuration = None,
        logs_crc_enabled: _LogsCrcEnabled = None,
        logs_compression_enabled: _LogsCompressionEnabled = None,
        logs_retention_max_files: _LogsRetentionMaxFiles = None,
        logs_retention_max_total_size: _LogsRetentionMaxTotalSize = None,
        logs_catalog_rotation_count: _LogsCatalogRotationCount = None,
        traces_rotation_max_file_size: _TracesRotationMaxFileSize = None,
        traces_rotation_max_entries: _TracesRotationMaxEntries = None,
        traces_rotation_max_file_duration: _TracesRotationMaxFileDuration = None,
        traces_crc_enabled: _TracesCrcEnabled = None,
        traces_compression_enabled: _TracesCompressionEnabled = None,
        traces_retention_max_files: _TracesRetentionMaxFiles = None,
        traces_retention_max_total_size: _TracesRetentionMaxTotalSize = None,
        traces_catalog_rotation_count: _TracesCatalogRotationCount = None,
        remote_storage_enabled: _RemoteStorageEnabled = None,
        remote_storage_uri: _RemoteStorageUri = None,
        journal_dir: _JournalDir = None,
        extra_yaml: _ExtraYaml = None,
    ) -> RunInfo:
        if get_agents(ctx).get(agent_id) is None:
            return unknown_agent(agent_id)
        if otlp_endpoint is not None:
            err = _endpoint_error(agent_id, otlp_endpoint)
            if err is not None:
                return err
        # "" is the disable sentinel (enabled: false), not a malformed address —
        # everything else must be a valid ip:port.
        if otlp_http_endpoint not in (None, ""):
            err = _endpoint_error(agent_id, otlp_http_endpoint, name="otlp_http_endpoint")
            if err is not None:
                return err
        if remote_storage_uri is not None and "://" not in remote_storage_uri:
            return agent_error(
                agent_id,
                f"remote_storage_uri must be an opendal URI like 'fs:///path' or 's3://bucket', got {remote_storage_uri!r}",
            )
        if extra_yaml is not None:
            err = _extra_yaml_error(agent_id, extra_yaml)
            if err is not None:
                return err
        cfg = OtelConfig(
            otlp_endpoint=otlp_endpoint,
            otlp_http_endpoint=otlp_http_endpoint,
            logs_rotation_max_file_size=logs_rotation_max_file_size,
            logs_rotation_max_entries=logs_rotation_max_entries,
            logs_rotation_max_file_duration=logs_rotation_max_file_duration,
            logs_crc_enabled=logs_crc_enabled,
            logs_compression_enabled=logs_compression_enabled,
            logs_retention_max_files=logs_retention_max_files,
            logs_retention_max_total_size=logs_retention_max_total_size,
            logs_catalog_rotation_count=logs_catalog_rotation_count,
            traces_rotation_max_file_size=traces_rotation_max_file_size,
            traces_rotation_max_entries=traces_rotation_max_entries,
            traces_rotation_max_file_duration=traces_rotation_max_file_duration,
            traces_crc_enabled=traces_crc_enabled,
            traces_compression_enabled=traces_compression_enabled,
            traces_retention_max_files=traces_retention_max_files,
            traces_retention_max_total_size=traces_retention_max_total_size,
            traces_catalog_rotation_count=traces_catalog_rotation_count,
            remote_storage_enabled=remote_storage_enabled,
            remote_storage_uri=remote_storage_uri,
            journal_dir=journal_dir,
            extra_yaml=extra_yaml,
        )
        if (clash := web_port_clash(cfg, get_agents(ctx).get(agent_id).port)) is not None:
            return agent_error(agent_id, clash)
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
