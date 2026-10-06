"""The tool boundary refuses an OTLP endpoint pinned to the agent's web port,
before it changes the stored agent spec."""

from __future__ import annotations

import types
from pathlib import Path

from mcp.server.fastmcp import FastMCP

from netdata_mcp import runtime
from netdata_mcp.agents import AgentRegistry
from netdata_mcp.tools import agents, otel_config


def _tool(register, name):
    mcp = FastMCP("t")
    register(mcp)
    return mcp._tool_manager.get_tool(name).fn


def _ctx(registry):
    runs = types.SimpleNamespace(get=lambda _agent_id: None)
    lifespan = types.SimpleNamespace(agents=registry, runs=runs)
    return types.SimpleNamespace(request_context=types.SimpleNamespace(lifespan_context=lifespan))


def _worktree(tmp_path):
    (tmp_path / "CMakeLists.txt").write_text("")
    return str(tmp_path)


async def test_otel_config_refuses_an_endpoint_on_the_declared_web_port(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: Path("/h")))
    registry = AgentRegistry()
    registry.declare("a", _worktree(tmp_path), "debug", port=19999)
    set_otel = _tool(otel_config.register, "netdata_agent_otel_config")
    res = await set_otel(_ctx(registry), "a", otlp_http_endpoint="127.0.0.1:19999")
    assert res.state == "error" and "uses the agent's web port 19999" in res.message
    assert registry.get("a").otel == runtime.OtelConfig()  # nothing stored
    res = await set_otel(_ctx(registry), "a", otlp_http_endpoint="[::1]:19999")
    assert res.state == "declared"  # a different address can share the port


async def test_declare_refuses_a_web_port_a_pinned_endpoint_uses(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: Path("/h")))
    registry = AgentRegistry()
    registry.declare("a", _worktree(tmp_path), "debug")
    registry.set_otel("a", runtime.OtelConfig(otlp_endpoint="127.0.0.1:5000"))
    declare = _tool(agents.register, "netdata_agent_declare")
    res = await declare(_ctx(registry), "a", _worktree(tmp_path), "debug", port=5000)
    assert res.state == "error" and "otlp_endpoint 127.0.0.1:5000" in res.message
    assert registry.get("a").port is None  # nothing stored
