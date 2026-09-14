"""Unit tests for the netdata_agent_otel_files tool: registration + the
result model. The network path (bearer mint + agentfn.call_function +
classify_status) is shared with otel_logs and covered by test_otel_logs.py."""

from __future__ import annotations

from mcp.server.fastmcp import FastMCP

from netdata_mcp.tools import otel_files
from netdata_mcp.tools.otel_files import OtelFilesResult


async def test_tool_registered():
    mcp = FastMCP("t")
    otel_files.register(mcp)
    tools = {t.name for t in await mcp.list_tools()}
    assert "netdata_agent_otel_files" in tools


async def test_tool_schema_takes_agent_id_and_timeout():
    mcp = FastMCP("t")
    otel_files.register(mcp)
    tool = next(t for t in await mcp.list_tools() if t.name == "netdata_agent_otel_files")
    props = tool.inputSchema["properties"]
    assert "agent_id" in props
    assert "agent_id" in tool.inputSchema["required"]
    assert "timeout" in props  # optional (has a default), so not in `required`
    assert "timeout" not in tool.inputSchema.get("required", [])


def test_result_model_defaults():
    out = OtelFilesResult(agent_id="a")
    assert out.agent_id == "a"
    assert out.response is None
    assert out.http_status is None
    assert out.error is None
    assert out.message == ""


def test_result_model_carries_inventory():
    inv = {"version": 1, "status": 200, "tenants": [{"tenant": "default", "wal": [], "sfst": [], "catalog": []}]}
    out = OtelFilesResult(agent_id="a", http_status=200, response=inv, message="ok")
    assert out.response["tenants"][0]["tenant"] == "default"
    assert out.http_status == 200
