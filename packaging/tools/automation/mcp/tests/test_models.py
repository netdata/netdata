from pathlib import Path

from netdata_mcp.agents import AgentSpec
from netdata_mcp.jobs import Job
from netdata_mcp.run import Run
from netdata_mcp.tools.models import agent_declared, agent_error, job_info, run_info, unknown_agent


def _job(state: str, n_lines: int, log_path: Path | None = None) -> Job:
    job = Job(
        id="job-1", kind="build", profile="debug", key="k", worktree="/wt",
        phases=[], log_path=log_path,
    )
    for i in range(n_lines):
        job.buffer.append(f"line{i}")
    job.state = state
    return job


def _line_count(text: str | None) -> int:
    return 0 if not text else len(text.splitlines())


def test_running_poll_returns_last_5_lines():
    info = job_info(_job("running", 200))
    assert _line_count(info.log_tail) == 5
    assert info.log_tail.endswith("line199")


def test_success_returns_last_10_lines():
    info = job_info(_job("succeeded", 200))
    assert _line_count(info.log_tail) == 10
    assert info.log_tail.endswith("line199")


def test_failure_returns_bounded_preview_not_full_log():
    info = job_info(_job("failed", 200))
    assert _line_count(info.log_tail) == 80  # bounded preview, NOT all 200 lines
    assert info.log_tail.endswith("line199")


def test_cancelled_returns_bounded_preview():
    info = job_info(_job("cancelled", 200))
    assert _line_count(info.log_tail) == 80


def test_log_file_path_is_exposed():
    info = job_info(_job("failed", 3, log_path=Path("/wt/build/.netdata-build.log")))
    assert info.log_file == "/wt/build/.netdata-build.log"


def test_log_file_none_when_unset():
    info = job_info(_job("running", 1))
    assert info.log_file is None


def test_no_output_yields_none():
    info = job_info(_job("running", 0))
    assert info.log_tail is None


def _run(state: str, n_lines: int = 0) -> Run:
    r = Run(
        agent_id="a", worktree="/wt", profile="debug", port=12345,
        run_dir=Path("/r"), conf_path=Path("/r/etc/netdata.conf"),
    )
    for i in range(n_lines):
        r.buffer.append(f"line{i}")
    r.state = state
    return r


def test_run_info_ready_exposes_url():
    info = run_info(_run("ready"))
    assert info.state == "ready"
    assert info.url == "http://127.0.0.1:12345"


def test_run_info_not_ready_has_no_url():
    assert run_info(_run("starting")).url is None


def test_run_info_failed_returns_80_line_tail():
    info = run_info(_run("failed", 200))
    assert info.log_tail is not None and len(info.log_tail.splitlines()) == 80


def test_run_info_running_returns_10_line_tail():
    info = run_info(_run("building", 200))
    assert info.log_tail is not None and len(info.log_tail.splitlines()) == 10


def test_run_info_exposes_claim_and_cloud_status():
    r = _run("ready")
    r.claimed, r.cloud_connected = True, False
    info = run_info(r)
    assert info.claimed is True and info.cloud_connected is False
    # unknown by default (e.g. before ready)
    assert run_info(_run("starting")).claimed is None


def test_agent_declared_unknown_and_error_states():
    assert agent_declared(AgentSpec(agent_id="a", worktree="/wt", profile="debug")).state == "declared"
    assert unknown_agent("x").state == "unknown"
    assert agent_error("x", "boom").state == "error"


def test_compile_commands_field_populated_only_when_db_exists(tmp_path):
    from netdata_mcp import buildcfg
    from netdata_mcp.tools import models

    assert models._compile_commands(None) is None        # no worktree
    assert models._compile_commands(str(tmp_path)) is None  # no DB yet

    db = buildcfg.compile_commands_path(str(tmp_path))
    db.parent.mkdir(parents=True, exist_ok=True)
    db.write_text("[]")
    assert models._compile_commands(str(tmp_path)) == str(db)
    # and it flows through the JobInfo factory
    job = Job(id="job-1", kind="build", profile="debug", key="k", worktree=str(tmp_path), phases=[])
    assert job_info(job).compile_commands == str(db)
