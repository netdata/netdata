"""Unit tests for netdata_mcp.journal: /proc parsing, PID resolution, and the
journalctl wrapper. No real /proc or journald — parsing is tested on synthetic
strings and PID resolution via monkeypatched tree/cmdline helpers."""

from __future__ import annotations

import asyncio
from typing import get_args

import pytest

from netdata_mcp import journal
from netdata_mcp.tools.logs import _Component


# ── _parse_ppid ───────────────────────────────────────────────

def test_parse_ppid_simple():
    assert journal._parse_ppid("1234 (netdata) S 1 1234 1234 0") == 1


def test_parse_ppid_comm_with_spaces():
    # comm may contain spaces; we parse after the LAST ')'.
    assert journal._parse_ppid("42 (otel plugin) S 7 42 ...") == 7


def test_parse_ppid_rejects_missing_paren():
    assert journal._parse_ppid("garbage with no paren") is None


def test_parse_ppid_rejects_truncated():
    assert journal._parse_ppid("42 (x) S") is None  # only state after ')'


def test_parse_ppid_rejects_non_integer():
    assert journal._parse_ppid("42 (x) S notapid 1") is None


# ── syslog_identifier ─────────────────────────────────────────

def test_syslog_identifier_mapping():
    assert journal.syslog_identifier("daemon") == "netdata"
    assert journal.syslog_identifier("supervisor") == "otel-plugin"
    assert journal.syslog_identifier("ledger") == "otel-plugin/ledger"
    assert journal.syslog_identifier("ingestor") == "otel-plugin/ingestor"
    assert journal.syslog_identifier("legacy-logs") == "otel-plugin/legacy-logs"


# ── resolve_pid (monkeypatched process tree) ──────────────────

# A fake netdata(100) -> otel-plugin supervisor(200) -> workers(201/202/203).
_FAKE_CMDLINES = {
    200: ["/x/otel-plugin", "1"],  # supervisor: netdata passes update_every as argv[1]
    201: ["/x/otel-plugin", "worker", "ledger", "--socket", "/t/l.sock"],
    202: ["/x/otel-plugin", "worker", "ingestor", "--socket", "/t/i.sock"],
    203: ["/x/otel-plugin", "worker", "legacy-logs", "--socket", "/t/g.sock"],
}


@pytest.fixture
def fake_tree(monkeypatch):
    monkeypatch.setattr(journal, "_descendant_pids", lambda root: set(_FAKE_CMDLINES))
    monkeypatch.setattr(journal, "_cmdline", lambda pid: _FAKE_CMDLINES.get(pid, []))


def test_resolve_pid_daemon_is_netdata_pid(fake_tree):
    # daemon doesn't walk the tree — it's the netdata PID itself.
    assert journal.resolve_pid(100, "daemon") == 100


def test_resolve_pid_supervisor(fake_tree):
    # the otel-plugin process whose argv is NOT `worker ...`
    assert journal.resolve_pid(100, "supervisor") == 200


def test_resolve_pid_workers(fake_tree):
    assert journal.resolve_pid(100, "ledger") == 201
    assert journal.resolve_pid(100, "ingestor") == 202
    assert journal.resolve_pid(100, "legacy-logs") == 203


def test_resolve_pid_worker_not_found(monkeypatch):
    monkeypatch.setattr(journal, "_descendant_pids", lambda root: {200})
    monkeypatch.setattr(journal, "_cmdline", lambda pid: ["/x/otel-plugin", "1"])
    assert journal.resolve_pid(100, "ledger") is None


# ── read_logs (monkeypatched subprocess) ──────────────────────

class _FakeProc:
    def __init__(self, out=b"", returncode=0, hang=False, kill_raises=False):
        self._out = out
        self.returncode = returncode
        self._hang = hang
        self._kill_raises = kill_raises
        self.killed = False

    async def communicate(self):
        # Hang only until killed — mirrors a real process whose communicate()
        # returns promptly once the process is SIGKILLed (so read_logs's
        # post-timeout drain communicate() doesn't block).
        if self._hang and not self.killed:
            await asyncio.sleep(3600)
        return (self._out, b"")

    def kill(self):
        self.killed = True
        if self._kill_raises:
            raise ProcessLookupError(3, "No such process")

    async def wait(self):
        return self.returncode


def _capture_exec(captured, proc):
    async def fake_exec(*cmd, **kwargs):
        captured["cmd"] = list(cmd)
        captured["kwargs"] = kwargs
        return proc
    return fake_exec


async def test_read_logs_builds_argv_with_pid_and_filters(monkeypatch):
    captured: dict = {}
    monkeypatch.setattr(asyncio, "create_subprocess_exec", _capture_exec(captured, _FakeProc(out=b"ok")))
    out = await journal.read_logs(
        identifier="otel-plugin/ledger", pid=4242, lines=50, grep="upload", priority="warning"
    )
    assert out == "ok"
    cmd = captured["cmd"]
    assert cmd[0] == "journalctl"
    assert "SYSLOG_IDENTIFIER=otel-plugin/ledger" in cmd
    assert "_PID=4242" in cmd
    assert cmd[cmd.index("-n") + 1] == "50"
    assert cmd[cmd.index("-p") + 1] == "warning"
    assert cmd[cmd.index("--grep") + 1] == "upload"
    assert captured["kwargs"].get("start_new_session") is True


async def test_read_logs_omits_pid_and_filters_when_absent(monkeypatch):
    captured: dict = {}
    monkeypatch.setattr(asyncio, "create_subprocess_exec", _capture_exec(captured, _FakeProc(out=b"")))
    await journal.read_logs(identifier="netdata", pid=None, lines=10, grep=None, priority=None)
    cmd = captured["cmd"]
    assert not any(c.startswith("_PID=") for c in cmd)
    assert "--grep" not in cmd
    assert "-p" not in cmd


async def test_read_logs_surfaces_nonzero_exit(monkeypatch):
    monkeypatch.setattr(
        asyncio, "create_subprocess_exec",
        _capture_exec({}, _FakeProc(out=b"No journal files were found", returncode=1)),
    )
    out = await journal.read_logs(identifier="netdata", pid=None, lines=5, grep=None, priority=None)
    assert "journalctl exited 1" in out
    assert "No journal files were found" in out


async def test_read_logs_times_out_and_kills(monkeypatch):
    proc = _FakeProc(hang=True)
    monkeypatch.setattr(asyncio, "create_subprocess_exec", _capture_exec({}, proc))
    monkeypatch.setattr(journal, "READ_TIMEOUT", 0.05)
    out = await journal.read_logs(identifier="netdata", pid=None, lines=5, grep=None, priority=None)
    assert "timed out" in out
    assert proc.killed is True


async def test_read_logs_timeout_when_process_already_exited(monkeypatch):
    # The process dies between the timeout and the kill -> ProcessLookupError;
    # read_logs must still return the timeout message, not raise.
    proc = _FakeProc(hang=True, kill_raises=True)
    monkeypatch.setattr(asyncio, "create_subprocess_exec", _capture_exec({}, proc))
    monkeypatch.setattr(journal, "READ_TIMEOUT", 0.05)
    out = await journal.read_logs(identifier="netdata", pid=None, lines=5, grep=None, priority=None)
    assert "timed out" in out


async def test_read_logs_missing_journalctl_never_raises(monkeypatch):
    async def boom(*cmd, **kwargs):
        raise FileNotFoundError("journalctl")
    monkeypatch.setattr(asyncio, "create_subprocess_exec", boom)
    out = await journal.read_logs(identifier="netdata", pid=None, lines=5, grep=None, priority=None)
    assert "journalctl not found" in out


# ── component enum stays in sync with journal.SOURCES ─────────

def test_component_literal_matches_sources():
    literal = get_args(_Component)[0]  # Annotated[Literal[...], Field] -> Literal[...]
    assert set(get_args(literal)) == set(journal.SOURCES)


# ── journal availability gate ─────────

def test_journald_socket_present_reflects_socket_presence(tmp_path, monkeypatch):
    sock = tmp_path / "socket"
    monkeypatch.setattr(journal, "_JOURNALD_SOCKET", sock)
    assert journal.journald_socket_present() is False
    sock.touch()
    assert journal.journald_socket_present() is True


def test_journald_socket_present_swallows_oserror(monkeypatch):
    # Path.exists() re-raises EACCES/EPERM; called at server startup, so it must
    # degrade to "absent" rather than abort boot.
    class _Boom:
        def exists(self):
            raise PermissionError("no access to /run/systemd/journal")

    monkeypatch.setattr(journal, "_JOURNALD_SOCKET", _Boom())
    assert journal.journald_socket_present() is False


def test_usable_requires_both_journald_and_journalctl(monkeypatch):
    cases = {
        "both present": (True, "/usr/bin/journalctl", True),
        "no journald socket": (False, "/usr/bin/journalctl", False),
        "no journalctl binary": (True, None, False),
        "neither": (False, None, False),
    }
    for name, (running, which, expected) in cases.items():
        monkeypatch.setattr(journal, "journald_socket_present", lambda running=running: running)
        monkeypatch.setattr(journal.shutil, "which", lambda _name, which=which: which)
        assert journal.usable() is expected, name
