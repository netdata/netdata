from mcp.server.fastmcp import FastMCP

from netdata_mcp.tools.vendored import register_forwarding_tool

SCHEMA = {
    "type": "object",
    "properties": {
        "agent_id": {"type": "string", "description": "Which running agent."},
        "metric": {"type": "string"},
        "after": {"type": "integer"},
    },
    "required": ["agent_id", "metric"],
}


async def test_vendored_tool_advertises_schema_and_forwards():
    mcp = FastMCP("t")
    calls = []

    async def forward(ctx, agent_id, arguments):
        calls.append((agent_id, arguments))
        return {"ok": agent_id}

    register_forwarding_tool(
        mcp, name="netdata_agent_query_metrics", description="d", input_schema=SCHEMA, forward=forward
    )

    # the LLM sees the vendored typed schema verbatim
    tools = await mcp.list_tools()
    tool = next(t for t in tools if t.name == "netdata_agent_query_metrics")
    assert tool.inputSchema == SCHEMA

    # a call splits out agent_id and forwards the rest
    await mcp._tool_manager.call_tool(
        "netdata_agent_query_metrics", {"agent_id": "a", "metric": "system.cpu", "after": -60}
    )
    assert calls == [("a", {"metric": "system.cpu", "after": -60})]


async def test_vendored_tool_forwards_with_only_agent_id():
    mcp = FastMCP("t")
    seen = []

    async def forward(ctx, agent_id, arguments):
        seen.append((agent_id, arguments))
        return None

    register_forwarding_tool(
        mcp, name="netdata_agent_list_nodes", description="d",
        input_schema={"type": "object", "properties": {"agent_id": {"type": "string"}}, "required": ["agent_id"]},
        forward=forward,
    )
    await mcp._tool_manager.call_tool("netdata_agent_list_nodes", {"agent_id": "a"})
    assert seen == [("a", {})]  # no extra args -> empty forward payload
