import types

from netdata_mcp.runner import LogBuffer
from netdata_mcp.tools import otel_effective_config
from netdata_mcp.tools.otel_effective_config import OtelEffectiveConfig, parse_effective


def test_parse_effective():
    cases = {
        "journal line": (
            ['2026-11-17T10:00:00+0000 host otel-plugin[42]: effective config: {"base_dir": "/x"}'],
            {"base_dir": "/x"},
        ),
        "last one wins": (
            ['INFO stock config: {"a": 1}', 'INFO effective config: {"a": 2}', 'INFO effective config: {"a": 3}'],
            {"a": 3},
        ),
        "stock and user lines are not the effective one": (['INFO user config: {"a": 1}'], None),
        "broken json is skipped": (['effective config: {"a": 1}', "effective config: {trunc"], {"a": 1}),
        "nothing": ([], None),
    }
    for name, (lines, want) in cases.items():
        assert parse_effective(lines) == want, name


def _call(monkeypatch, run, journald):
    monkeypatch.setattr(otel_effective_config, "get_runs", lambda ctx: types.SimpleNamespace(get=lambda _id: run))
    monkeypatch.setattr(otel_effective_config.journal, "journald_socket_present", lambda: journald)
    from mcp.server.fastmcp import FastMCP

    mcp = FastMCP("t")
    otel_effective_config.register(mcp)
    fn = mcp._tool_manager.get_tool("netdata_agent_otel_effective_config").fn
    return fn(types.SimpleNamespace(), "a")


async def test_reads_the_run_buffer_without_journald(monkeypatch):
    buf = LogBuffer()
    for line in ("starting", 'INFO stock config: {"a": 1}', 'INFO effective config: {"a": 2}'):
        buf.append(line)
    run = types.SimpleNamespace(state="ready", buffer=buf, pid=None, created_wall=0.0)
    out = await _call(monkeypatch, run, journald=False)
    assert out == OtelEffectiveConfig(agent_id="a", source="run_logs", effective={"a": 2}, message="ok")


async def test_reads_the_journal_from_the_run_start(monkeypatch):
    seen = {}

    async def fake_read(**kw):
        seen.update(kw)
        return 'x otel-plugin[7]: ERROR parsing /r/etc/otel.yaml: unknown field `tracez`'

    monkeypatch.setattr(otel_effective_config.journal, "read_logs", fake_read)
    run = types.SimpleNamespace(state="ready", buffer=LogBuffer(), pid=None, created_wall=1700000000.5)
    out = await _call(monkeypatch, run, journald=True)
    assert seen == {
        "identifier": "otel-plugin", "pid": None, "lines": 500,
        "grep": "config", "priority": None, "since": 1700000000.5,
    }
    assert out.source == "journal" and out.effective is None
    assert out.config_log == 'x otel-plugin[7]: ERROR parsing /r/etc/otel.yaml: unknown field `tracez`'
    assert out.error is not None and "refused" in out.error


async def test_unknown_agent(monkeypatch):
    out = await _call(monkeypatch, None, journald=False)
    assert out.error is not None and out.source is None
