from mcp.server.fastmcp import FastMCP

from netdata_mcp.tools import otel_service_map
from netdata_mcp.tools.otel_service_map import build_payload


def test_build_payload():
    cases = {
        "defaults": (dict(info=False, after=None, before=None, tenant=None), {}),
        "window": (dict(info=False, after=-600, before=0, tenant="t1"), {"after": -600, "before": 0, "tenant": "t1"}),
        "info ignores the window": (dict(info=True, after=-600, before=None, tenant="t1"), {"info": True}),
    }
    for name, (kw, want) in cases.items():
        assert build_payload(**kw) == want, name


async def test_tool_registered():
    mcp = FastMCP("t")
    otel_service_map.register(mcp)
    tool = next(t for t in await mcp.list_tools() if t.name == "netdata_agent_otel_service_map")
    assert set(tool.inputSchema["properties"]) == {"agent_id", "info", "after", "before", "tenant", "timeout"}
    assert tool.inputSchema["required"] == ["agent_id"]
