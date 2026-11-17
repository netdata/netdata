import json
import types
from pathlib import Path

from netdata_mcp.runner import LogBuffer
from netdata_mcp.tools import otel_effective_config
from netdata_mcp.tools.otel_effective_config import OtelEffectiveConfig, parse_effective, scope_to_run


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


def _env(pid, run_dir):
    env = {"user_config_dir": f"{run_dir}/etc", "run_dir": f"{run_dir}/run"}
    return f"T host otel-plugin[{pid}]: netdata environment: {json.dumps(env, separators=(',', ':'))}"


def test_scope_to_run_keeps_only_the_agents_supervisor():
    lines = [
        _env(7, "/r/a"),
        _env(8, "/r/b"),
        'T host otel-plugin[8]: effective config: {"base_dir": "/r/b/lib/otel"}',
        "T host otel-plugin[7]: failed to load configuration: parsing /r/a/etc/otel.yaml: unknown field `tracez`",
        "garbage without a pid",
    ]
    assert scope_to_run(lines, "/r/a/etc") == [lines[0], lines[3]]
    assert scope_to_run(lines, "/r/c/etc") == []


async def test_journal_ignores_other_agents_and_keeps_the_refusal(monkeypatch):
    # Another agent logs its effective config after this run started; this
    # run's supervisor refused its config and exited.
    seen = {}
    refusal = "T host otel-plugin[7]: failed to load configuration: parsing /r/a/etc/otel.yaml: unknown field `tracez`"

    async def fake_read(**kw):
        seen.update(kw)
        return "\n".join([
            _env(7, "/r/a"), refusal,
            _env(8, "/r/b"), 'T host otel-plugin[8]: effective config: {"base_dir": "/r/b/lib/otel"}',
        ])

    monkeypatch.setattr(otel_effective_config.journal, "read_logs", fake_read)
    run = types.SimpleNamespace(
        state="ready", buffer=LogBuffer(), pid=None, created_wall=1700000000.5, run_dir=Path("/r/a")
    )
    out = await _call(monkeypatch, run, journald=True)
    assert seen == {
        "identifier": "otel-plugin", "pid": None, "lines": 500,
        "grep": "config", "priority": None, "since": 1700000000.5,
    }
    assert out.source == "journal" and out.effective is None
    assert out.config_log == "\n".join([_env(7, "/r/a"), refusal])
    assert out.error is not None and "refused" in out.error


async def test_journal_returns_the_agents_own_effective_config(monkeypatch):
    async def fake_read(**kw):
        return "\n".join([
            _env(7, "/r/a"), 'T host otel-plugin[7]: effective config: {"base_dir": "/r/a/lib/otel"}',
            _env(8, "/r/b"), 'T host otel-plugin[8]: effective config: {"base_dir": "/r/b/lib/otel"}',
        ])

    monkeypatch.setattr(otel_effective_config.journal, "read_logs", fake_read)
    run = types.SimpleNamespace(state="ready", buffer=LogBuffer(), pid=None, created_wall=0.0, run_dir=Path("/r/a"))
    out = await _call(monkeypatch, run, journald=True)
    assert out == OtelEffectiveConfig(
        agent_id="a", source="journal", effective={"base_dir": "/r/a/lib/otel"}, message="ok"
    )


async def test_unknown_agent(monkeypatch):
    out = await _call(monkeypatch, None, journald=False)
    assert out.error is not None and out.source is None
