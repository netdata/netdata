import asyncio
from pathlib import Path

import pytest_asyncio

from netdata_mcp import buildcfg, runtime
from netdata_mcp import run as runmod
from netdata_mcp.locks import build_dir_lock
from netdata_mcp.run import RunRegistry


async def _always_ready(port):
    return True


async def _never_ready(port):
    return False


@pytest_asyncio.fixture
async def reg(monkeypatch, tmp_path):
    # isolate run dirs under tmp; default fakes (overridable per test).
    # worktree (cwd for build/launch) must exist -> use tmp_path itself.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    monkeypatch.setattr(buildcfg, "needs_configure", lambda wt, p: False)
    monkeypatch.setattr(buildcfg, "install_command", lambda wt: ["sh", "-c", "true"])
    monkeypatch.setattr(runtime, "install_bin", lambda wt: Path("/bin/sh"))
    monkeypatch.setattr(runtime, "launch_command", lambda b, port, conf: ["sh", "-c", "sleep 30"])
    r = RunRegistry()
    try:
        yield r
    finally:
        await r.stop_all()


async def _wait(run, states, timeout=8.0):
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while run.state not in states and loop.time() < deadline:
        await asyncio.sleep(0.02)
    return run.state


async def test_run_reaches_ready_then_stops(reg, tmp_path):
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    assert await _wait(run, {"ready", "failed", "stopped"}) == "ready"
    assert run.port and run.current_phase == "launch"
    stopped = await reg.stop("a")
    assert stopped.state == "stopped"
    assert run._task is not None and run._task.done()


async def test_build_failure_marks_failed(reg, tmp_path, monkeypatch):
    monkeypatch.setattr(buildcfg, "install_command", lambda wt: ["sh", "-c", "exit 1"])
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    assert await _wait(run, {"ready", "failed", "stopped"}) == "failed"
    assert "install failed" in (run.error or "")


async def test_stop_before_ready(reg, tmp_path):
    # never becomes ready (probe False) but launch is alive; stop must reap it
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
    await _wait(run, {"starting"})
    stopped = await reg.stop("a", wait=3.0)
    assert stopped.state == "stopped"
    assert run._task.done()


async def test_start_is_idempotent_while_live(reg, tmp_path):
    run1, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run1, {"ready"})
    run2, outcome = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    assert run2 is run1 and outcome == "already-running"  # same live run reused


async def test_restart_relaunches_live_run(reg, tmp_path):
    # restart=True on a live agent must tear it down and start a fresh run
    # (the rebuild path) so source edits are picked up.
    run1, o1 = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    assert o1 == "started"
    await _wait(run1, {"ready"})
    run2, o2 = await reg.start("a", str(tmp_path), "debug", restart=True, probe=_always_ready)
    assert o2 == "restarted"
    assert run2 is not run1  # a fresh run, not the reused live one
    assert run1.state == "stopped"  # the old run was torn down
    assert await _wait(run2, {"ready", "failed", "stopped"}) == "ready"


async def test_restart_on_idle_agent_just_starts(reg, tmp_path):
    # restart=True with no live run is a plain start, not a "restart".
    run, outcome = await reg.start("a", str(tmp_path), "debug", restart=True, probe=_always_ready)
    assert outcome == "started"
    assert await _wait(run, {"ready"}) == "ready"


async def test_stop_while_queued_on_build_lock_is_prompt(reg, tmp_path):
    # A run blocked acquiring the build-dir lock must stop promptly (without
    # waiting to acquire) and never build/launch. The test holds the lock for
    # the whole body, so the stop completing here proves the run never acquired.
    lockfile = buildcfg.lock_file(str(tmp_path))
    async with build_dir_lock(lockfile):
        run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
        await asyncio.sleep(0.3)
        assert not run.done  # blocked acquiring the build-dir lock
        stopped = await reg.stop("a", wait=1.0)
        assert stopped is not None and stopped.state == "stopped"
        assert run._proc is None  # never built/launched


async def test_concurrent_restart_serializes_no_orphan(reg, tmp_path):
    # Two concurrent restart=True must serialize (per-agent start lock): each
    # stops the prior run before creating the next, so no run is left orphaned
    # alive. Without the lock both would see the same live run, both create a
    # fresh run, and the loser of the _runs[agent_id] overwrite would leak.
    run0, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run0, {"ready"})
    (rA, oA), (rB, oB) = await asyncio.gather(
        reg.start("a", str(tmp_path), "debug", restart=True, probe=_always_ready),
        reg.start("a", str(tmp_path), "debug", restart=True, probe=_always_ready),
    )
    assert oA == "restarted" and oB == "restarted"
    current = reg.get("a")
    # every run other than the one now registered must have been reaped, i.e.
    # nothing is left running in the background unreachable via stop().
    for r in (run0, rA, rB):
        if r is not current:
            assert await _wait(r, {"stopped", "failed"}) in {"stopped", "failed"}


async def test_readiness_timeout_escalates_to_sigkill_and_fails(reg, tmp_path, monkeypatch):
    monkeypatch.setattr(runmod, "_READINESS_TIMEOUT", 0.3)
    monkeypatch.setattr(runmod, "_TIMEOUT_KILL_GRACE", 0.3)
    # group leader traps SIGTERM and stays alive holding the pipe; only the
    # SIGKILL escalation can reap it -> run must reach "failed", not hang.
    monkeypatch.setattr(
        runtime, "launch_command",
        lambda b, port, conf: ["sh", "-c", "trap '' TERM; while true; do sleep 0.2; done"],
    )
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
    assert await _wait(run, {"failed", "stopped"}, timeout=10) == "failed"
    assert "ready within" in (run.error or "")
    # state flips to "failed" before the reap; the task must then wind down
    # promptly (proves the SIGKILL escalation didn't hang).
    for _ in range(100):
        if run._task is not None and run._task.done():
            break
        await asyncio.sleep(0.05)
    assert run._task is not None and run._task.done()


async def test_configure_phase_runs_when_needed(reg, tmp_path, monkeypatch):
    # exercise the needs_configure=True path (the fixture defaults it to False)
    monkeypatch.setattr(buildcfg, "needs_configure", lambda wt, p: True)
    monkeypatch.setattr(buildcfg, "configure_command", lambda wt, p: ["sh", "-c", "true"])
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    assert await _wait(run, {"ready", "failed"}) == "ready"


async def test_run_logs_are_incremental(reg, tmp_path):
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run, {"ready"})
    first = run.buffer.read(0)
    assert first.next_offset > 0 and first.text  # build + launch markers present
    nxt = run.buffer.read(first.next_offset)
    assert nxt.text == ""  # nothing repeated; the fake launch emits no further output


async def test_build_is_serialized_per_worktree(reg, tmp_path, monkeypatch):
    # Two agents from one worktree share the single build dir -> one build-dir
    # file lock -> their installs serialize (do not interleave).
    marker = tmp_path / "order.txt"
    monkeypatch.setattr(
        buildcfg, "install_command",
        lambda wt: ["sh", "-c", f"echo S >> {marker}; sleep 0.3; echo E >> {marker}"],
    )
    r1, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    r2, _ = await reg.start("b", str(tmp_path), "debug", probe=_always_ready)
    await _wait(r1, {"ready", "failed"})
    await _wait(r2, {"ready", "failed"})
    # serialized installs do not interleave: S,E,S,E (not S,S,E,E)
    assert marker.read_text().split() == ["S", "E", "S", "E"]


async def test_run_captures_launch_returncode(reg, tmp_path, monkeypatch):
    # netdata's exit code is recorded on the Run once the launch ends.
    monkeypatch.setattr(runtime, "launch_command", lambda b, port, conf: ["sh", "-c", "exit 7"])
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
    assert await _wait(run, {"ready", "failed", "stopped"}) == "failed"
    assert run.returncode == 7


async def _spy_run_command(monkeypatch):
    """Replace run.py's run_command with a spy that records the launch env kwarg."""
    captured = {}
    real = runmod.run_command

    async def spy(cmd, cwd, sink, on_spawn=None, env=None):
        captured["env"] = env
        return await real(cmd, cwd, sink, on_spawn=on_spawn, env=env)

    monkeypatch.setattr(runmod, "run_command", spy)
    return captured


async def test_launch_passes_claim_env_when_token_present(reg, tmp_path, monkeypatch):
    monkeypatch.setenv("NETDATA_CLAIM_TOKEN", "tok-123")
    monkeypatch.setenv("NETDATA_CLAIM_ROOMS", "room-1")
    captured = await _spy_run_command(monkeypatch)
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run, {"ready", "failed"})
    assert captured["env"]["NETDATA_CLAIM_TOKEN"] == "tok-123"
    assert captured["env"]["NETDATA_CLAIM_ROOMS"] == "room-1"
    assert any("claiming to Cloud as mcp-a" in line for line in run.buffer.read(0).text.splitlines())


async def test_launch_unclaimed_without_token(reg, tmp_path, monkeypatch):
    monkeypatch.delenv("NETDATA_CLAIM_TOKEN", raising=False)
    captured = await _spy_run_command(monkeypatch)
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run, {"ready", "failed"})
    # token explicitly blanked so a stray inherited NETDATA_CLAIM_TOKEN can't claim
    assert captured["env"] == {"NETDATA_CLAIM_TOKEN": ""}
    assert any("running unclaimed" in line for line in run.buffer.read(0).text.splitlines())


async def test_run_returncode_is_negative_when_killed_by_signal(reg, tmp_path, monkeypatch):
    # locks the RunInfo.returncode "negative = killed by signal" contract.
    monkeypatch.setattr(runtime, "launch_command", lambda b, port, conf: ["sh", "-c", "kill -9 $$"])
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
    assert await _wait(run, {"ready", "failed", "stopped"}) == "failed"
    assert run.returncode is not None and run.returncode < 0


async def test_run_wait_status_long_polls_and_handles_unknown(reg, tmp_path):
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
    await _wait(run, {"building", "starting"})
    r = await reg.wait_status("a", timeout=0.2, poll=0.05)  # still coming up -> returns it
    assert r is run and r.state in ("building", "starting")
    assert await reg.wait_status("zzz", timeout=0.1) is None  # unknown agent


async def test_wait_status_reports_cloud_status_when_ready(reg, tmp_path, monkeypatch):
    async def fake_cloud(port, timeout=2.0):
        return (True, False)  # claimed, but ACLK not yet connected
    monkeypatch.setattr(runtime, "cloud_status", fake_cloud)
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run, {"ready"})
    r = await reg.wait_status("a", timeout=0.2)
    assert r.claimed is True and r.cloud_connected is False


async def test_wait_status_does_not_clobber_cloud_status_on_transient_failure(reg, tmp_path, monkeypatch):
    seq = [(True, True), (None, None)]  # first poll observes connected; second fails

    async def fake(port, timeout=2.0):
        return seq.pop(0) if seq else (None, None)

    monkeypatch.setattr(runtime, "cloud_status", fake)
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_always_ready)
    await _wait(run, {"ready"})
    r1 = await reg.wait_status("a", timeout=0.2)
    assert r1.claimed is True and r1.cloud_connected is True
    r2 = await reg.wait_status("a", timeout=0.2)  # (None, None) must not reset the known values
    assert r2.claimed is True and r2.cloud_connected is True


async def test_run_wait_status_returns_terminal_immediately(reg, tmp_path, monkeypatch):
    # a run that already ended must return at once, not wait out the timeout.
    monkeypatch.setattr(runtime, "launch_command", lambda b, port, conf: ["sh", "-c", "exit 1"])
    run, _ = await reg.start("a", str(tmp_path), "debug", probe=_never_ready)
    assert await _wait(run, {"failed", "stopped"}) == "failed"
    r = await reg.wait_status("a", timeout=5.0, poll=0.05)  # long timeout, but terminal -> immediate
    assert r is run and r.done
