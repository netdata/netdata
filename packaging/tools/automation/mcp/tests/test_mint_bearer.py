"""Unit tests for the netdata_agent_mint_bearer tool: registration, schema,
result model, and the durable contract that the description embeds the
Playwright injection recipe (so future sessions don't re-derive it). The mint
path itself (bearer.resolve_bearer) is covered by test_bearer.py."""

from __future__ import annotations

import types

from mcp.server.fastmcp import FastMCP

from netdata_mcp.tools import mint_bearer
from netdata_mcp.tools.mint_bearer import MintBearerResult


def _ctx_with(run):
    """A fake Context whose run registry returns `run` for any agent_id."""
    reg = types.SimpleNamespace(get=lambda _id: run)
    return types.SimpleNamespace(_reg=reg)


async def test_tool_registered():
    mcp = FastMCP("t")
    mint_bearer.register(mcp)
    tools = {t.name for t in await mcp.list_tools()}
    assert "netdata_agent_mint_bearer" in tools


async def test_tool_schema_takes_agent_id_and_timeout():
    mcp = FastMCP("t")
    mint_bearer.register(mcp)
    tool = next(t for t in await mcp.list_tools() if t.name == "netdata_agent_mint_bearer")
    props = tool.inputSchema["properties"]
    assert "agent_id" in props
    assert "agent_id" in tool.inputSchema["required"]
    assert "timeout" in props  # optional (has a default), so not in `required`
    assert "timeout" not in tool.inputSchema.get("required", [])


async def test_description_embeds_playwright_injection_recipe():
    # The whole point of the tool: the description must carry the how-to so an
    # LLM can run the workflow without rediscovering it. Guard that contract.
    mcp = FastMCP("t")
    mint_bearer.register(mcp)
    tool = next(t for t in await mcp.list_tools() if t.name == "netdata_agent_mint_bearer")
    desc = tool.description
    assert "Playwright" in desc
    assert "browser_run_code_unsafe" in desc
    assert "setExtraHTTPHeaders" in desc


def test_result_model_defaults():
    out = MintBearerResult(agent_id="a")
    assert out.agent_id == "a"
    assert out.bearer is None
    assert out.error is None
    assert out.message == ""


def test_result_model_carries_bearer():
    out = MintBearerResult(agent_id="a", bearer="tok", message="ok")
    assert out.bearer == "tok"
    assert out.error is None


# ── tool body (_mint_bearer) branches ───────────────────────────────────────────
async def test_mint_returns_bearer_on_success(monkeypatch):
    monkeypatch.setattr(mint_bearer, "get_runs", lambda ctx: ctx._reg)

    async def fake_resolve(port, *, timeout):
        return "THE-BEARER", None

    monkeypatch.setattr(mint_bearer.bearer, "resolve_bearer", fake_resolve)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await mint_bearer._mint_bearer(_ctx_with(run), "a", 30)
    assert out.bearer == "THE-BEARER"
    assert out.error is None


async def test_mint_errors_when_agent_unknown(monkeypatch):
    monkeypatch.setattr(mint_bearer, "get_runs", lambda ctx: ctx._reg)
    out = await mint_bearer._mint_bearer(_ctx_with(None), "x", 30)
    assert out.bearer is None
    assert "not ready" in out.error and "unknown" in out.error


async def test_mint_errors_when_agent_not_ready(monkeypatch):
    monkeypatch.setattr(mint_bearer, "get_runs", lambda ctx: ctx._reg)
    run = types.SimpleNamespace(state="building", port=None)
    out = await mint_bearer._mint_bearer(_ctx_with(run), "a", 30)
    assert out.bearer is None
    assert "not ready" in out.error and "building" in out.error


async def test_mint_errors_when_mint_fails(monkeypatch):
    monkeypatch.setattr(mint_bearer, "get_runs", lambda ctx: ctx._reg)

    async def fake_resolve(port, *, timeout):
        return None, "cloud unreachable"

    monkeypatch.setattr(mint_bearer.bearer, "resolve_bearer", fake_resolve)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await mint_bearer._mint_bearer(_ctx_with(run), "a", 30)
    assert out.bearer is None
    assert "could not mint a bearer" in out.error and "cloud unreachable" in out.error


async def test_mint_error_hint_not_duplicated_when_token_missing(monkeypatch):
    # When resolve_bearer already names the missing token, the hint must not repeat it.
    monkeypatch.setattr(mint_bearer, "get_runs", lambda ctx: ctx._reg)

    async def fake_resolve(port, *, timeout):
        return None, "NETDATA_CLOUD_TOKEN is not set"

    monkeypatch.setattr(mint_bearer.bearer, "resolve_bearer", fake_resolve)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await mint_bearer._mint_bearer(_ctx_with(run), "a", 30)
    assert out.error.count("NETDATA_CLOUD_TOKEN") == 1  # once, inside the (err) parens


async def test_mint_error_hint_added_when_token_present(monkeypatch):
    # An unrelated mint failure still gets the full "set NETDATA_CLOUD_TOKEN" hint.
    monkeypatch.setattr(mint_bearer, "get_runs", lambda ctx: ctx._reg)

    async def fake_resolve(port, *, timeout):
        return None, "agent identity incomplete"

    monkeypatch.setattr(mint_bearer.bearer, "resolve_bearer", fake_resolve)
    run = types.SimpleNamespace(state="ready", port=45999)
    out = await mint_bearer._mint_bearer(_ctx_with(run), "a", 30)
    assert "Ensure NETDATA_CLOUD_TOKEN is set" in out.error
