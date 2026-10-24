import asyncio
import types

from netdata_mcp import streams
from netdata_mcp.tools.otel_push import OtelPushResult, _run_push


def _ctx(run):
    """A minimal Context whose get_runs(ctx) resolves to a registry returning `run`."""
    runs = types.SimpleNamespace(get=lambda _agent_id: run)
    lifespan = types.SimpleNamespace(runs=runs)
    return types.SimpleNamespace(request_context=types.SimpleNamespace(lifespan_context=lifespan))


def _ready_run():
    return types.SimpleNamespace(state="ready", otlp_endpoint="127.0.0.1:1", worktree="/wt")


def test_run_push_rejects_not_ready_without_running():
    # No run / not ready → error result, and the cmd_builder is NEVER invoked
    # (no generator spawned for an unready agent).
    called = False

    def builder(_ep):
        nonlocal called
        called = True
        return ["cargo"]

    res = asyncio.run(_run_push(_ctx(None), "a", 5, builder, timeout=10))
    assert isinstance(res, OtelPushResult)
    assert res.success is False and "not ready" in (res.error or "")
    assert res.message == "agent not ready"
    assert called is False


def test_run_push_maps_failure(monkeypatch):
    monkeypatch.setattr(streams, "run_synth", _async_return((1, "boom", None)))
    res = asyncio.run(_run_push(_ctx(_ready_run()), "a", 5, lambda ep: ["cargo"], timeout=10))
    assert res.success is False and res.returncode == 1
    assert res.log_tail == "boom" and "code 1" in (res.error or "")
    assert res.otel_endpoint == "127.0.0.1:1"


def test_run_push_maps_success(monkeypatch):
    monkeypatch.setattr(streams, "run_synth", _async_return((0, "ok", None)))
    res = asyncio.run(_run_push(_ctx(_ready_run()), "a", 7, lambda ep: ["cargo"], timeout=10))
    assert res.success is True and res.returncode == 0 and res.count == 7
    assert res.message == "sent 7 records"


def _async_return(value):
    async def _f(*_args, **_kwargs):
        return value

    return _f
