"""Schema tests for netdata_agent_declare: the optional, bounded port."""

from __future__ import annotations

from mcp.server.fastmcp import FastMCP

from netdata_mcp.tools import agents


async def _declare_tool():
    mcp = FastMCP("t")
    agents.register(mcp)
    return next(t for t in await mcp.list_tools() if t.name == "netdata_agent_declare")


async def test_declare_takes_an_optional_bounded_port():
    tool = await _declare_tool()
    props = tool.inputSchema["properties"]
    assert "port" in props
    assert "port" not in tool.inputSchema.get("required", [])
    bounds = next(v for v in props["port"]["anyOf"] if v.get("type") == "integer")
    assert (bounds["minimum"], bounds["maximum"]) == (1, 65535)
