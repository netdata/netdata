"""Generic job-control tools: status (long-poll), logs, cancel.

These operate over any job_id regardless of which domain started it, leaning on
the single shared registry.  Per-domain *start* tools live in their own modules.
"""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from ._common import get_registry
from .models import JobInfo, JobLogs, job_info, state_message, unknown_job

_JobId = Annotated[str, Field(description="Job id returned by a *_start tool.")]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_job_status",
        description=(
            "Get the status of a configure/build job started by a *_start tool. "
            "This call long-polls: it waits up to ~8s for the job to finish before "
            "returning, so call it repeatedly until `state` is no longer 'running' "
            "(it becomes 'succeeded', 'failed', or 'cancelled'). Returns a recent "
            "log tail; use netdata_job_logs for the full paged output."
        ),
    )
    async def netdata_job_status(ctx: Context, job_id: _JobId) -> JobInfo:
        registry = get_registry(ctx)
        job = await registry.wait_status(job_id)
        if job is None:
            return unknown_job(job_id)
        return job_info(job, message=state_message(job))

    @mcp.tool(
        name="netdata_job_logs",
        description=(
            "Fetch a job's output incrementally. Pass offset=0 the first time, then "
            "pass back the returned `next_offset` to read only new lines. `truncated` "
            "is true if older lines were evicted before your offset."
        ),
    )
    async def netdata_job_logs(
        ctx: Context,
        job_id: _JobId,
        offset: Annotated[int, Field(description="Line offset from a prior call; 0 to start.", ge=0)] = 0,
    ) -> JobLogs:
        registry = get_registry(ctx)
        result = registry.logs(job_id, offset)
        if result is None:
            return JobLogs(
                job_id=job_id,
                state="unknown",
                text="",
                next_offset=0,
                truncated=False,
                message="No such job. Jobs are in-memory and do not survive a server restart.",
            )
        job, sl = result
        return JobLogs(
            job_id=job_id,
            state=job.state,
            text=sl.text,
            next_offset=sl.next_offset,
            truncated=sl.truncated,
            message="Some earlier lines were evicted from the buffer." if sl.truncated else "",
        )

    @mcp.tool(
        name="netdata_job_cancel",
        description=(
            "Cancel a running job (terminates the underlying process). Waits briefly "
            "for it to wind down and returns the final state. No-op if already finished."
        ),
    )
    async def netdata_job_cancel(ctx: Context, job_id: _JobId) -> JobInfo:
        registry = get_registry(ctx)
        job = await registry.cancel(job_id)
        if job is None:
            return unknown_job(job_id)
        return job_info(job, message=state_message(job))
