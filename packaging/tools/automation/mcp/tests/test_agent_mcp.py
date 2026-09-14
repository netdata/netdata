import types

from mcp.server.fastmcp import FastMCP

from netdata_mcp.agent_tools import AGENT_TOOLS
from netdata_mcp.tools import agent_mcp


def test_with_agent_id_injects_required_agent_id():
    schema = {"type": "object", "properties": {"metric": {"type": "string"}}, "required": ["metric"]}
    out = agent_mcp._with_agent_id(schema)
    assert out["properties"]["agent_id"]["type"] == "string"
    assert out["required"] == ["agent_id", "metric"]
    assert schema["required"] == ["metric"]  # input not mutated


async def test_register_exposes_all_agent_tools_with_typed_schema():
    mcp = FastMCP("t")
    agent_mcp.register(mcp)
    tools = {t.name: t for t in await mcp.list_tools()}
    assert len(AGENT_TOOLS) == 13
    for agent_tool in AGENT_TOOLS:
        t = tools[f"netdata_agent_{agent_tool}"]
        assert "agent_id" in t.inputSchema["properties"]  # typed schema, agent_id injected
        assert "agent_id" in t.inputSchema["required"]


def test_structured_fields_picks_array_and_object():
    schema = {
        "type": "object",
        "properties": {
            "dimensions": {"type": "array"},
            "group_by": {"type": "object"},
            "metric": {"type": "string"},
            "after": {"type": "integer"},
        },
    }
    assert agent_mcp._structured_fields(schema) == {"dimensions", "group_by"}


def test_coerce_structured_decodes_json_strings_only_for_structured():
    out = agent_mcp._coerce_structured(
        {"dimensions": '["read","write"]', "metric": "system.cpu", "after": -60}, {"dimensions"}
    )
    assert out["dimensions"] == ["read", "write"]  # JSON string decoded
    assert out["metric"] == "system.cpu" and out["after"] == -60  # non-structured untouched


def test_coerce_structured_leaves_native_and_undecodable():
    out = agent_mcp._coerce_structured({"dimensions": ["a"], "labels": "not-json"}, {"dimensions", "labels"})
    assert out["dimensions"] == ["a"]  # already native
    assert out["labels"] == "not-json"  # undecodable -> left for the agent to reject


def _ctx_with(run):
    reg = types.SimpleNamespace(get=lambda _id: run)
    return types.SimpleNamespace(_reg=reg)


async def test_forward_unknown_agent(monkeypatch):
    monkeypatch.setattr(agent_mcp, "get_runs", lambda ctx: ctx._reg)
    out = await agent_mcp._forward_to_agent(_ctx_with(None), "x", "query_metrics", {})
    assert "No such agent" in out


async def test_forward_not_ready_agent(monkeypatch):
    monkeypatch.setattr(agent_mcp, "get_runs", lambda ctx: ctx._reg)
    run = types.SimpleNamespace(state="building", port=None)
    out = await agent_mcp._forward_to_agent(_ctx_with(run), "a", "query_metrics", {})
    assert "not ready" in out and "building" in out


async def test_forward_ready_agent_mints_and_calls_core(monkeypatch):
    monkeypatch.setattr(agent_mcp, "get_runs", lambda ctx: ctx._reg)
    monkeypatch.setattr(agent_mcp.bearer, "cloud_token", lambda: "cloud-tok")

    async def fake_resolve(port, *, timeout):
        return "minted-bearer", None

    monkeypatch.setattr(agent_mcp.bearer, "resolve_bearer", fake_resolve)
    seen = {}

    async def fake_call(base_url, tool, arguments, *, bearer):
        seen.update(base_url=base_url, tool=tool, arguments=arguments, bearer=bearer)
        return '{"ok": true}'

    monkeypatch.setattr(agent_mcp.agentmcp, "call_agent_tool", fake_call)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await agent_mcp._forward_to_agent(_ctx_with(run), "a", "query_metrics", {"metric": "system.cpu"})
    assert out == '{"ok": true}'
    assert seen == {
        "base_url": "http://127.0.0.1:45999",
        "tool": "query_metrics",
        "arguments": {"metric": "system.cpu"},
        "bearer": "minted-bearer",
    }


async def test_forward_hard_errors_without_cloud_token(monkeypatch):
    # Bearer invariant: no NETDATA_CLOUD_TOKEN → hard error, never an anonymous call.
    monkeypatch.setattr(agent_mcp, "get_runs", lambda ctx: ctx._reg)
    monkeypatch.setattr(agent_mcp.bearer, "cloud_token", lambda: "")
    called = False

    async def fake_call(*args, **kwargs):
        nonlocal called
        called = True
        return "x"

    monkeypatch.setattr(agent_mcp.agentmcp, "call_agent_tool", fake_call)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await agent_mcp._forward_to_agent(_ctx_with(run), "a", "query_metrics", {})
    assert "NETDATA_CLOUD_TOKEN is not set" in out
    assert called is False  # no anonymous fallback


async def test_forward_hard_errors_on_mint_failure(monkeypatch):
    # Token present but the mint fails → hard error, still no anonymous call.
    monkeypatch.setattr(agent_mcp, "get_runs", lambda ctx: ctx._reg)
    monkeypatch.setattr(agent_mcp.bearer, "cloud_token", lambda: "cloud-tok")

    async def fake_resolve(port, *, timeout):
        return None, "cloud unreachable"

    monkeypatch.setattr(agent_mcp.bearer, "resolve_bearer", fake_resolve)
    called = False

    async def fake_call(*args, **kwargs):
        nonlocal called
        called = True
        return "x"

    monkeypatch.setattr(agent_mcp.agentmcp, "call_agent_tool", fake_call)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await agent_mcp._forward_to_agent(_ctx_with(run), "a", "query_metrics", {})
    assert "Could not obtain a Netdata Cloud bearer" in out and "cloud unreachable" in out
    assert called is False
