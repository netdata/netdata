"""Structured tool responses (the MCP-facing wire shapes)."""

from __future__ import annotations

from typing import TYPE_CHECKING

from pydantic import BaseModel, Field

from .. import buildcfg
from ..jobs import Job, StartResult

if TYPE_CHECKING:
    from ..agents import AgentSpec
    from ..run import Run

_CC_DESC = (
    "Path to the build's compile_commands.json (the clangd database) at "
    "<worktree>/build/, once configured. clangd discovers it natively. Editor/"
    "clangd errors that contradict a successful build are stale-database false "
    "positives — trust the build."
)


def _compile_commands(worktree: str | None) -> str | None:
    if not worktree:
        return None
    db = buildcfg.compile_commands_path(worktree)
    return str(db) if db.is_file() else None

# How much log to attach to a JobInfo, by job state. Polls stay cheap; success
# gives a little more; a failed/cancelled job gets a larger preview tail. The
# COMPLETE log is on disk at `log_file` (and via paged netdata_job_logs) — we do
# NOT inline the full log, which would overflow client tool-output limits.
_POLL_TAIL_LINES = 5
_SUCCESS_TAIL_LINES = 10
_FAILURE_TAIL_LINES = 80


def _log_tail_for(job: Job) -> str:
    if job.state == "running":
        return job.buffer.tail(_POLL_TAIL_LINES)
    if job.state == "succeeded":
        return job.buffer.tail(_SUCCESS_TAIL_LINES)
    # failed / cancelled: a generous preview; full log is at job.log_path
    return job.buffer.tail(_FAILURE_TAIL_LINES)


class JobInfo(BaseModel):
    """State of a single job, returned by start / status / cancel tools."""

    job_id: str = Field(description="Job identifier; pass to netdata_job_* tools. Empty on input errors.")
    kind: str = Field(description="Job kind: 'configure', 'build', or 'unknown'/'error'.")
    state: str = Field(description="running | succeeded | failed | cancelled | unknown | error")
    profile: str | None = Field(default=None, description="Build profile, if applicable.")
    worktree: str | None = Field(default=None, description="Target worktree, if applicable.")
    current_phase: str | None = Field(default=None, description="Phase currently executing (e.g. 'configure', 'build').")
    returncode: int | None = Field(default=None, description="Exit code once a phase or the job finished.")
    elapsed_seconds: float = Field(default=0.0, description="Seconds since the job started.")
    outcome: str | None = Field(default=None, description="For start calls: started | deduped | busy.")
    log_tail: str | None = Field(default=None, description="Recent output preview: ~5 lines while running, ~10 on success, ~80 on failure/cancel.")
    log_file: str | None = Field(default=None, description="Path to the COMPLETE build log on disk. On failure, grep/read it for every error; it is unbounded and never truncated.")
    compile_commands: str | None = Field(default=None, description=_CC_DESC)
    message: str = Field(default="", description="Human-readable hint about what to do next.")


class JobLogs(BaseModel):
    """A page of a job's output, addressable by a caller-held offset."""

    job_id: str
    state: str = Field(description="Job state at read time.")
    text: str = Field(description="New output lines since `offset`, newline-joined.")
    next_offset: int = Field(description="Pass this back as `offset` on the next call to continue.")
    truncated: bool = Field(description="True if older lines were evicted before `offset` and skipped.")
    message: str = Field(default="")


def job_info(job: Job, *, outcome: str | None = None, message: str = "") -> JobInfo:
    return JobInfo(
        job_id=job.id,
        kind=job.kind,
        state=job.state,
        profile=job.profile,
        worktree=job.worktree,
        current_phase=job.current_phase,
        returncode=job.returncode,
        elapsed_seconds=round(job.elapsed(), 1),
        outcome=outcome,
        log_tail=_log_tail_for(job) or None,
        log_file=str(job.log_path) if job.log_path is not None else None,
        compile_commands=_compile_commands(job.worktree),
        message=message,
    )


def unknown_job(job_id: str) -> JobInfo:
    return JobInfo(
        job_id=job_id,
        kind="unknown",
        state="unknown",
        message="No such job. Jobs are in-memory and do not survive a server restart.",
    )


def input_error(message: str) -> JobInfo:
    return JobInfo(job_id="", kind="error", state="error", message=message)


def start_message(result: StartResult, kind: str) -> str:
    job = result.job
    if result.outcome == "started":
        return f"{kind} job {job.id} started. Poll netdata_job_status({job.id!r}) until state is not 'running'."
    if result.outcome == "deduped":
        return f"An identical {kind} is already running as {job.id}; reusing it. Poll netdata_job_status({job.id!r})."
    # busy
    return (
        f"That build dir is busy with {job.kind} job {job.id} (profile {job.profile}). "
        f"Wait for it via netdata_job_status({job.id!r}), or netdata_job_cancel({job.id!r}) first."
    )


class RunInfo(BaseModel):
    """State of an agent run, returned by the declare/run tools."""

    agent_id: str
    state: str = Field(description="declared | building | starting | ready | stopped | failed | unknown | error")
    profile: str | None = None
    worktree: str | None = None
    port: int | None = Field(default=None, description="Assigned loopback port.")
    url: str | None = Field(default=None, description="http://127.0.0.1:<port> once the agent is ready.")
    otlp_endpoint: str | None = Field(default=None, description="Where the otel plugin listens for OTLP/gRPC data (host:port); send test logs here.")
    current_phase: str | None = Field(default=None, description="configure | install | launch.")
    elapsed_seconds: float = 0.0
    returncode: int | None = Field(default=None, description="netdata's exit code once a launched agent has stopped/failed (negative = killed by signal).")
    claimed: bool | None = Field(default=None, description="Whether the agent claimed to Netdata Cloud (known once ready; null otherwise).")
    cloud_connected: bool | None = Field(default=None, description="Whether the agent is online in Cloud (ACLK). May lag 'claimed' by seconds; reported, never waited on.")
    run_dir: str | None = Field(default=None, description="Isolated runtime dir; netdata's daemon.log is at <run_dir>/log/.")
    log_tail: str | None = None
    compile_commands: str | None = Field(default=None, description=_CC_DESC)
    message: str = ""


class RunLogs(BaseModel):
    agent_id: str
    state: str = Field(description="Agent state at read time.")
    text: str = Field(description="New output lines since `offset`, newline-joined.")
    next_offset: int = Field(description="Pass this back as `offset` on the next call to continue.")
    truncated: bool = Field(description="True if older lines were evicted before `offset` and skipped.")
    message: str = Field(default="", description="Human-readable hint; empty on success.")


class AgentLogs(BaseModel):
    agent_id: str
    component: str = Field(
        description="Which part of the agent: daemon | supervisor | ledger | ingestor | legacy-logs."
    )
    syslog_identifier: str = Field(
        description="systemd SYSLOG_IDENTIFIER the query was scoped to (e.g. 'netdata', 'otel-plugin', 'otel-plugin/ledger')."
    )
    pid: int | None = Field(
        default=None,
        description=(
            "Process PID the journal query was scoped to (daemon/supervisor/worker). "
            "None when the agent isn't running or the process couldn't be resolved; "
            "results are then identifier-scoped and may span other agents."
        ),
    )
    text: str = Field(
        description="journalctl output, newline-joined; a bracketed status note (e.g. timeout, "
        "journalctl missing) on a read failure; or empty when the agent isn't found (see message)."
    )
    message: str = Field(
        default="", description="Human-readable scoping/resolution note; empty when the query was cleanly PID-scoped."
    )


def unknown_agent_logs(agent_id: str, component: str, identifier: str) -> AgentLogs:
    return AgentLogs(
        agent_id=agent_id,
        component=component,
        syslog_identifier=identifier,
        pid=None,
        text="",
        message="No such agent (in-memory; does not survive a server restart).",
    )


def run_info(run: Run, *, message: str = "") -> RunInfo:
    ready = run.state == "ready"
    tail = run.buffer.tail(80 if run.state in ("failed", "stopped") else 10)
    return RunInfo(
        agent_id=run.agent_id,
        state=run.state,
        profile=run.profile,
        worktree=run.worktree,
        port=run.port,
        url=(f"http://127.0.0.1:{run.port}" if ready else None),
        otlp_endpoint=run.otlp_endpoint or None,
        current_phase=run.current_phase,
        elapsed_seconds=round(run.elapsed(), 1),
        returncode=run.returncode,
        claimed=run.claimed,
        cloud_connected=run.cloud_connected,
        run_dir=str(run.run_dir),
        log_tail=tail or None,
        compile_commands=_compile_commands(run.worktree),
        message=message,
    )


def agent_declared(spec: AgentSpec, *, message: str = "") -> RunInfo:
    return RunInfo(agent_id=spec.agent_id, state="declared", profile=spec.profile, worktree=spec.worktree, message=message)


def unknown_agent(agent_id: str) -> RunInfo:
    return RunInfo(agent_id=agent_id, state="unknown", message="No such agent. Declare it with netdata_agent_declare first.")


def agent_error(agent_id: str, message: str) -> RunInfo:
    return RunInfo(agent_id=agent_id, state="error", message=message)


def state_message(job: Job) -> str:
    if job.state == "running":
        phase = f" (phase: {job.current_phase})" if job.current_phase else ""
        return f"Still running{phase}. Call netdata_job_status again to keep waiting."
    if job.state == "succeeded":
        return f"Succeeded in {round(job.elapsed(), 1)}s."
    if job.state == "failed":
        why = job.error or f"exit code {job.returncode}"
        where = f" Full log: {job.log_path}" if job.log_path is not None else f" See netdata_job_logs({job.id!r})"
        return f"Failed: {why}.{where} (grep it for 'error' to find every failure)."
    if job.state == "cancelled":
        return "Cancelled."
    return ""
