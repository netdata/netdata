"""Otel domain: set the otel-plugin config an agent launches with.

A declared agent carries an :class:`~netdata_mcp.runtime.OtelConfig`; this tool
replaces it. It is written as the agent's user ``otel.yaml`` on the next
``netdata_run_start`` / restart, so the edit-config -> restart -> re-query loop
is how an LLM forces storage edge cases (rotation, retention) over small,
deterministic corpora. What the plugin actually applied is read back with
``netdata_agent_otel_effective_config``.
"""

from __future__ import annotations

from typing import Annotated

import yaml

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from ..otel_settings import OtelSettings
from ..runtime import OtelConfig, web_port_clash
from ._common import get_agents, get_runs
from .models import RunInfo, agent_declared, agent_error, run_info, unknown_agent

_AgentId = Annotated[str, Field(description="The declared agent to configure.")]
_Config = Annotated[
    OtelSettings | None,
    Field(description=(
        "The user otel.yaml as a nested object, same keys as the plugin's stock otel.yaml "
        "(receivers, auth, metrics, remote_storage, logs, traces). Omit for the stock config."
    )),
]
_ExtraYaml = Annotated[
    str | None,
    Field(description=(
        "Raw YAML mapping deep-merged over the generated file (wins on conflicts). Only for "
        "configs the typed 'config' refuses on purpose: unknown or misplaced keys for "
        "strict-config refusal tests. A config the plugin refuses keeps it down; the refusal "
        "shows in netdata_agent_otel_effective_config."
    )),
]


def _extra_yaml_error(agent_id: str, value: str) -> RunInfo | None:
    """An error RunInfo unless ``value`` parses to a mapping (or nothing).
    Semantic validity is deliberately not checked: feeding the plugin a config
    it refuses is a supported test."""
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
            "Set the otel-plugin config for a declared agent, applied on the next "
            "netdata_run_start (or restart=true). REPLACES any prior config: pass the whole "
            "config each call; an omitted key keeps the plugin's stock value. 'config' has the "
            "shape of otel.yaml, e.g. {\"traces\": {\"rotation\": {\"default\": {\"max_entries\": "
            "10}}, \"retention\": {\"default\": {\"max_files\": 3}}}, \"remote_storage\": "
            "{\"enabled\": true}, \"receivers\": {\"otlp\": {\"protocols\": {\"http\": "
            "{\"enabled\": false}}}}}. Rotation/retention are maps keyed by tenant ('default' "
            "covers the rest). Sizes: '25MB' or bytes; durations: '7 days', '30m'. Harness rules: "
            "base_dir is pinned under the agent's run dir; an unset gRPC/HTTP endpoint gets a free "
            "loopback port; the HTTP listener is on unless enabled=false; remote_storage enabled "
            "without uri uses a per-agent fs:// dir; an endpoint on the agent's declared web port "
            "is refused. Small rotation/retention/catalog values force multi-file, eviction and "
            "upload edge cases over a small corpus. Check what the plugin applied with "
            "netdata_agent_otel_effective_config once the agent is ready."
        ),
    )
    async def netdata_agent_otel_config(
        ctx: Context,
        agent_id: _AgentId,
        config: _Config = None,
        extra_yaml: _ExtraYaml = None,
    ) -> RunInfo:
        agents = get_agents(ctx)
        spec = agents.get(agent_id)
        if spec is None:
            return unknown_agent(agent_id)
        if extra_yaml is not None and (err := _extra_yaml_error(agent_id, extra_yaml)) is not None:
            return err
        cfg = OtelConfig(settings=config or OtelSettings(), extra_yaml=extra_yaml)
        if (clash := web_port_clash(cfg, spec.port)) is not None:
            return agent_error(agent_id, clash)
        spec = agents.set_otel(agent_id, cfg)
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
