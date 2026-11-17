import types

from netdata_mcp.tools import gated_function
from netdata_mcp.tools.gated_function import FunctionResult, call_gated_function


def _ctx_with(run, monkeypatch):
    reg = types.SimpleNamespace(get=lambda _id: run)
    monkeypatch.setattr(gated_function, "get_runs", lambda ctx: reg)
    return types.SimpleNamespace()


def _fake_call(seen, status=200, data=None, err=None):
    async def call(base_url, function, payload, *, timeout, bearer):
        seen.update(base_url=base_url, function=function, payload=payload, timeout=timeout, bearer=bearer)
        return status, data if data is not None else {"ok": True}, err

    return call


async def test_not_ready_agent(monkeypatch):
    ctx = _ctx_with(types.SimpleNamespace(state="building", port=None), monkeypatch)
    out = await call_gated_function(ctx, "a", "otel-traces", {"info": {}}, 60)
    assert out == FunctionResult(
        agent_id="a", function="otel-traces", request={"info": {}},
        error="Agent 'a' is not ready (state=building). "
        "Start it with netdata_run_start and poll netdata_run_status until 'ready'.",
    )


async def test_unknown_agent(monkeypatch):
    ctx = _ctx_with(None, monkeypatch)
    out = await call_gated_function(ctx, "a", "otel-traces", {}, 60)
    assert out.error is not None and "state=unknown" in out.error


async def test_mints_bearer_and_calls(monkeypatch):
    ctx = _ctx_with(types.SimpleNamespace(state="ready", port=45999), monkeypatch)
    monkeypatch.setattr(gated_function.bearer, "cloud_token", lambda: "cloud-tok")
    mint = {}

    async def fake_resolve(port, *, timeout):
        mint.update(port=port, timeout=timeout)
        return "minted", None

    monkeypatch.setattr(gated_function.bearer, "resolve_bearer", fake_resolve)
    seen = {}
    monkeypatch.setattr(gated_function.agentfn, "call_function", _fake_call(seen))
    out = await call_gated_function(ctx, "a", "otel-traces", {"info": {}}, 120)
    assert mint == {"port": 45999, "timeout": 20}
    assert seen == {
        "base_url": "http://127.0.0.1:45999", "function": "otel-traces",
        "payload": {"info": {}}, "timeout": 120, "bearer": "minted",
    }
    assert out == FunctionResult(
        agent_id="a", function="otel-traces",
        endpoint="http://127.0.0.1:45999/api/v3/function?function=otel-traces&timeout=120",
        http_status=200, request={"info": {}}, response={"ok": True}, message="ok",
    )


async def test_mint_failure_is_hard(monkeypatch):
    ctx = _ctx_with(types.SimpleNamespace(state="ready", port=45999), monkeypatch)
    monkeypatch.setattr(gated_function.bearer, "cloud_token", lambda: "cloud-tok")

    async def fake_resolve(port, *, timeout):
        return None, "cloud said no"

    monkeypatch.setattr(gated_function.bearer, "resolve_bearer", fake_resolve)
    seen = {}
    monkeypatch.setattr(gated_function.agentfn, "call_function", _fake_call(seen))
    out = await call_gated_function(ctx, "a", "otel-logs", {}, 60)
    assert seen == {}
    assert out.error is not None and "cloud said no" in out.error


async def test_no_cloud_token_calls_anonymously(monkeypatch):
    ctx = _ctx_with(types.SimpleNamespace(state="ready", port=45999), monkeypatch)
    monkeypatch.setattr(gated_function.bearer, "cloud_token", lambda: "")
    seen = {}
    monkeypatch.setattr(
        gated_function.agentfn, "call_function", _fake_call(seen, status=412, data={"status": 412})
    )
    out = await call_gated_function(ctx, "a", "otel-logs", {}, 60)
    assert seen["bearer"] is None
    assert out.http_status == 412 and out.error is not None and "SIGNED_ID" in out.error
