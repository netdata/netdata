import asyncio
import os

import pytest
import pytest_asyncio

from netdata_mcp.jobs import JobRegistry, Phase
from netdata_mcp.locks import build_dir_lock


@pytest_asyncio.fixture
async def reg():
    """A registry that is fully drained on teardown.

    Draining within the test's own event loop guarantees no live subprocess
    transport survives loop close — a stray one deadlocks the next test's loop.
    """
    registry = JobRegistry()
    try:
        yield registry
    finally:
        await registry.cancel_all()


async def _wait_done(reg: JobRegistry, job_id: str, timeout: float = 10.0):
    job = reg.get(job_id)
    assert job is not None
    deadline = asyncio.get_running_loop().time() + timeout
    while not job.done and asyncio.get_running_loop().time() < deadline:
        await asyncio.sleep(0.02)
    return job


async def _wait_spawned(job, timeout: float = 5.0):
    """Poll until the subprocess is actually spawned (avoids fragile fixed sleeps)."""
    deadline = asyncio.get_running_loop().time() + timeout
    while job._proc is None and asyncio.get_running_loop().time() < deadline:
        await asyncio.sleep(0.02)
    return job._proc


def _phase(name: str, script: str) -> Phase:
    return Phase(name=name, cmd=["sh", "-c", script], cwd=".")


async def test_start_runs_to_success(reg):
    res = await reg.start(kind="build", profile="debug", key="k1", worktree="/wt", phases=[_phase("p", "echo hi")])
    assert res.outcome == "started"
    job = await _wait_done(reg, res.job.id)
    assert job.state == "succeeded"
    assert job.returncode == 0


async def test_dedup_identical_request(reg):
    r1 = await reg.start(kind="build", profile="debug", key="k", worktree="/wt", phases=[_phase("p", "sleep 0.4")])
    r2 = await reg.start(kind="build", profile="debug", key="k", worktree="/wt", phases=[_phase("p", "sleep 0.4")])
    assert r2.outcome == "deduped"
    assert r2.job.id == r1.job.id
    await _wait_done(reg, r1.job.id)


async def test_lockfile_wires_through_and_serializes(reg, tmp_path):
    # Two jobs with DIFFERENT keys (so neither is deduped/busy -> both start) but
    # the SAME lockfile must serialize via the build-dir file lock. Proves
    # start() passes lockfile to Job (regression: it previously did not, so the
    # build/configure tools raised TypeError) and that the lock is enforced for
    # build jobs, not only for the run domain.
    lockfile = tmp_path / ".netdata-mcp-build.lock"
    marker = tmp_path / "order.txt"
    marker.write_text("")
    script = f"echo S >> {marker}; sleep 0.3; echo E >> {marker}"
    r1 = await reg.start(kind="build", profile="debug", key="k1", worktree="/wt",
                         phases=[_phase("p", script)], lockfile=lockfile)
    r2 = await reg.start(kind="build", profile="debug", key="k2", worktree="/wt",
                         phases=[_phase("p", script)], lockfile=lockfile)
    assert r1.outcome == "started" and r2.outcome == "started"
    assert r1.job.lockfile == lockfile  # wiring: start() -> Job
    await _wait_done(reg, r1.job.id)
    await _wait_done(reg, r2.job.id)
    assert r1.job.state == "succeeded" and r2.job.state == "succeeded"
    # serialized: S,E,S,E (not the interleaved S,S,E,E of an unenforced lock)
    assert marker.read_text().split() == ["S", "E", "S", "E"]


async def test_cancel_while_queued_on_lock_is_prompt(reg, tmp_path):
    # A job queued on a held build-dir lock must cancel promptly (without waiting
    # to acquire) and never run its phase. The test holds the lock for the whole
    # body, so the cancel completing here proves the job never acquired it.
    lockfile = tmp_path / ".netdata-mcp-build.lock"
    async with build_dir_lock(lockfile):
        res = await reg.start(kind="build", profile="debug", key="kw", worktree="/wt",
                              phases=[_phase("p", "echo NOPE")], lockfile=lockfile)
        await asyncio.sleep(0.3)
        assert not res.job.done  # blocked waiting for the lock
        cancelled = await reg.cancel(res.job.id, wait=1.0)
        assert cancelled is not None and cancelled.state == "cancelled"
        assert res.job._proc is None  # the phase never spawned


async def test_busy_when_different_kind_on_same_key(reg):
    r1 = await reg.start(kind="configure", profile="debug", key="k", worktree="/wt", phases=[_phase("p", "sleep 0.4")])
    r2 = await reg.start(kind="build", profile="debug", key="k", worktree="/wt", phases=[_phase("p", "echo x")])
    assert r2.outcome == "busy"
    assert r2.job.id == r1.job.id
    await _wait_done(reg, r1.job.id)


async def test_different_keys_run_in_parallel(reg):
    r1 = await reg.start(kind="build", profile="debug", key="ka", worktree="/a", phases=[_phase("p", "echo a")])
    r2 = await reg.start(kind="build", profile="debug", key="kb", worktree="/b", phases=[_phase("p", "echo b")])
    assert r1.outcome == "started" and r2.outcome == "started"
    assert r1.job.id != r2.job.id
    await _wait_done(reg, r1.job.id)
    await _wait_done(reg, r2.job.id)


async def test_unknown_job_handled_gracefully(reg):
    assert reg.get("nope") is None
    assert await reg.wait_status("nope") is None
    assert reg.logs("nope", 0) is None
    assert await reg.cancel("nope") is None


async def test_failed_phase_stops_remaining_phases(reg):
    phases = [_phase("a", "exit 2"), _phase("b", "echo should-not-run")]
    res = await reg.start(kind="build", profile="debug", key="k", worktree="/wt", phases=phases)
    job = await _wait_done(reg, res.job.id)
    assert job.state == "failed"
    assert job.returncode == 2
    result = reg.logs(res.job.id, 0)
    assert result is not None
    assert "should-not-run" not in result[1].text


async def test_cancel_running_job(reg):
    res = await reg.start(kind="build", profile="debug", key="k", worktree="/wt", phases=[_phase("p", "sleep 30")])
    job = await reg.cancel(res.job.id)
    assert job is not None
    assert job.state == "cancelled"


async def test_open_log_failure_warns_and_job_still_runs(reg, tmp_path):
    if os.getuid() == 0:
        pytest.skip("directory permissions do not constrain root")
    ro = tmp_path / "ro"
    ro.mkdir()
    ro.chmod(0o500)  # no write: creating the log's parent dir will fail
    log = ro / "sub" / ".netdata-build.log"
    try:
        res = await reg.start(
            kind="build", profile="debug", key="k", worktree="/wt",
            phases=[_phase("p", "echo hi")], log_path=log,
        )
        job = await _wait_done(reg, res.job.id)
        assert job.state == "succeeded"  # job runs even if the log file can't open
        assert "could not open log file" in job.buffer.read(0).text
    finally:
        ro.chmod(0o700)  # let tmp cleanup remove it


async def test_cancel_during_running_phase_idle_on_stdout(reg):
    # Regression: a process idle on stdout at cancel time used to leave the task
    # parked (state stayed "running") because only the direct child got SIGTERM.
    # Process-group kill must reap it and reach "cancelled".
    res = await reg.start(
        kind="build", profile="debug", key="k", worktree="/wt",
        phases=[_phase("p", "sleep 30")],
    )
    proc = await _wait_spawned(res.job)
    assert proc is not None and proc.returncode is None  # spawned and idle on stdout
    job = await reg.cancel(res.job.id, wait=5.0)
    assert job.state == "cancelled"
    assert job._task is not None and job._task.done()


async def test_cancel_reaps_grandchild_holding_pipe(reg):
    # The outer shell backgrounds a child that inherits stdout; if only the
    # parent were killed, that child would keep the pipe open and stall the
    # reader. Group kill must reap the whole tree.
    res = await reg.start(
        kind="build", profile="debug", key="k", worktree="/wt",
        phases=[_phase("p", "sleep 30 & sleep 30")],
    )
    await _wait_spawned(res.job)
    job = await reg.cancel(res.job.id, wait=5.0)
    assert job.state == "cancelled"
    assert job._task is not None and job._task.done()


async def test_cancel_escalates_to_sigkill_when_sigterm_ignored(reg):
    # The shell traps SIGTERM and stays alive holding the stdout pipe; only the
    # SIGKILL escalation can reap it.
    res = await reg.start(
        kind="build", profile="debug", key="k", worktree="/wt",
        phases=[_phase("p", "trap '' TERM; while true; do sleep 1; done")],
    )
    await _wait_spawned(res.job)
    job = await reg.cancel(res.job.id, wait=2.0)
    assert job.state == "cancelled"
    assert job._task is not None and job._task.done()


async def test_cancel_all_terminates_all_running_jobs(reg):
    r1 = await reg.start(
        kind="build", profile="debug", key="ka", worktree="/a",
        phases=[_phase("p", "sleep 30")],
    )
    r2 = await reg.start(
        kind="build", profile="debug", key="kb", worktree="/b",
        phases=[_phase("p", "sleep 30")],
    )
    await _wait_spawned(r1.job)
    await _wait_spawned(r2.job)
    await reg.cancel_all(wait=5.0)
    assert r1.job.done and r2.job.done


async def test_log_file_is_written_with_full_output(reg, tmp_path):
    log = tmp_path / "build" / ".netdata-build.log"
    res = await reg.start(
        kind="build",
        profile="debug",
        key="k",
        worktree="/wt",
        phases=[_phase("p", "printf 'x\\ny\\nz\\n'")],
        log_path=log,
    )
    await _wait_done(reg, res.job.id)
    assert log.is_file()
    content = log.read_text()
    assert "x" in content and "y" in content and "z" in content
    assert "[phase: p]" in content  # phase markers are teed to the file too


async def test_wait_status_long_poll_returns_on_completion(reg):
    res = await reg.start(kind="build", profile="debug", key="k", worktree="/wt", phases=[_phase("p", "sleep 0.2")])
    job = await reg.wait_status(res.job.id, timeout=5.0, poll=0.02)
    assert job is not None
    assert job.done
