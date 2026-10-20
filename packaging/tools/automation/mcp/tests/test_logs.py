"""Unit tests for the netdata_agent_logs tool's non-subprocess logic: the
PID-resolution note disambiguation and the unknown-agent path."""

from __future__ import annotations

from mcp.server.fastmcp import FastMCP

from netdata_mcp import journal
from netdata_mcp.tools import logs
from netdata_mcp.tools.logs import _resolution_note
from netdata_mcp.tools.models import unknown_agent_logs


def test_note_empty_when_pid_resolved():
    assert _resolution_note(netdata_pid=100, pid=201, component="ledger") == ""


def test_note_agent_not_running():
    note = _resolution_note(netdata_pid=None, pid=None, component="ledger")
    assert "not running" in note
    assert "identifier-scoped" in note


def test_note_running_but_process_not_found():
    note = _resolution_note(netdata_pid=100, pid=None, component="ingestor")
    assert "running" in note
    assert "'ingestor'" in note
    assert "not found" in note
    # The "older server build" caveat does not belong here (a registry run was
    # launched by the current server).
    assert "older server build" not in note


async def test_tool_registered_when_journal_usable(monkeypatch):
    monkeypatch.setattr(journal, "usable", lambda: True)
    mcp = FastMCP("t")
    logs.register(mcp)
    assert "netdata_agent_logs" in {t.name for t in await mcp.list_tools()}


async def test_default_component_is_daemon(monkeypatch):
    # The default must be the netdata daemon (what "the agent's logs" means with
    # no component), not an otel-plugin worker. Pins the LLM-facing contract.
    monkeypatch.setattr(journal, "usable", lambda: True)
    mcp = FastMCP("t")
    logs.register(mcp)
    tool = next(t for t in await mcp.list_tools() if t.name == "netdata_agent_logs")
    assert tool.inputSchema["properties"]["component"]["default"] == "daemon"


async def test_tool_not_registered_when_journal_unusable(monkeypatch):
    # No journalctl / no journald -> nothing to read; the tool must not appear,
    # so callers fall back to netdata_run_logs / the run-dir log files.
    monkeypatch.setattr(journal, "usable", lambda: False)
    mcp = FastMCP("t")
    logs.register(mcp)
    assert "netdata_agent_logs" not in {t.name for t in await mcp.list_tools()}


def test_unknown_agent_logs_shape():
    out = unknown_agent_logs("ghost", "ledger", "otel-plugin/ledger")
    assert out.agent_id == "ghost"
    assert out.component == "ledger"
    assert out.syslog_identifier == "otel-plugin/ledger"
    assert out.pid is None
    assert out.text == ""
    assert "No such agent" in out.message
