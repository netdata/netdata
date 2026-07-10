from netdata_mcp.agent_tools import AGENT_TOOLS


def test_vendored_snapshot_is_well_formed():
    # the 13 tools captured from the agent's /mcp; each carries a forwarding schema
    assert len(AGENT_TOOLS) == 13
    assert "query_metrics" in AGENT_TOOLS and "execute_function" in AGENT_TOOLS
    for name, spec in AGENT_TOOLS.items():
        assert set(spec) == {"description", "inputSchema"}, name
        schema = spec["inputSchema"]
        assert schema.get("type") == "object" and "properties" in schema, name
