"""Agent-MCP wrapper: re-expose a running agent's own ``/mcp`` tools as native
build-MCP tools, each keyed by ``agent_id`` and forwarded to that agent's ``/mcp``.

The agent's 13 tool schemas are vendored (``agent_tools.AGENT_TOOLS``); each is
registered as ``netdata_agent_<name>`` with an injected ``agent_id`` (D1=A, baked).
A call resolves the agent's port via ``RunRegistry`` (must be ``ready``) and
forwards opaquely (D4). No auth needed on a localhost agent.
"""

from __future__ import annotations

import copy
import json
from typing import Any

from mcp.server.fastmcp import Context, FastMCP

from .. import agentmcp
from ..agent_tools import AGENT_TOOLS
from ._common import get_runs
from .vendored import register_forwarding_tool

_AGENT_ID_SCHEMA = {
    "type": "string",
    "description": "Agent id from netdata_agent_declare; the agent must be 'ready'.",
}


def _with_agent_id(schema: dict[str, Any]) -> dict[str, Any]:
    """Return a copy of a vendored input schema with a required ``agent_id`` added."""
    out = copy.deepcopy(schema)
    out.setdefault("properties", {})["agent_id"] = _AGENT_ID_SCHEMA
    required = out.get("required", [])
    if "agent_id" not in required:
        out["required"] = ["agent_id", *required]
    return out


def _coerce_structured(arguments: dict[str, Any], structured: set[str]) -> dict[str, Any]:
    """JSON-decode string values for array/object params (FastMCP's pre_parse_json
    parity: some clients send a structured arg as a JSON string). Undecodable values
    are left as-is for the agent to reject."""
    if not structured:
        return arguments
    out = dict(arguments)
    for name in structured:
        value = out.get(name)
        if isinstance(value, str):
            try:
                out[name] = json.loads(value)
            except (ValueError, TypeError):
                pass
    return out


def _structured_fields(schema: dict[str, Any]) -> set[str]:
    props = schema.get("properties") or {}
    return {n for n, p in props.items() if isinstance(p, dict) and p.get("type") in ("array", "object")}


async def _forward_to_agent(ctx: Context, agent_id: str, agent_tool: str, arguments: dict[str, Any]) -> str:
    """Resolve ``agent_id`` to a ready agent and forward ``agent_tool`` to its /mcp."""
    run = get_runs(ctx).get(agent_id)
    if run is None:
        return f"No such agent {agent_id!r}. Declare it (netdata_agent_declare) and start it first."
    if run.state != "ready" or not run.port:
        return (
            f"Agent {agent_id!r} is not ready (state: {run.state}). "
            "Start it and poll netdata_run_status until 'ready'."
        )
    return await agentmcp.call_agent_tool(f"http://127.0.0.1:{run.port}", agent_tool, arguments)


def register(mcp: FastMCP) -> None:
    for agent_tool, spec in AGENT_TOOLS.items():
        structured = _structured_fields(spec["inputSchema"])

        def _forwarder(agent_tool: str = agent_tool, structured: set[str] = structured):  # bind per-iteration
            async def forward(ctx: Context, agent_id: str, arguments: dict[str, Any]) -> str:
                return await _forward_to_agent(ctx, agent_id, agent_tool, _coerce_structured(arguments, structured))

            return forward

        register_forwarding_tool(
            mcp,
            name=f"netdata_agent_{agent_tool}",
            description=f"[runs on a ready built agent, by agent_id] {spec['description']}",
            input_schema=_with_agent_id(spec["inputSchema"]),
            forward=_forwarder(),
        )
