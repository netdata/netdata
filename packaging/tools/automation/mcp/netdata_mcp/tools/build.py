"""Build domain: start a (configure-if-needed +) ninja build job for a worktree."""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .. import buildcfg
from ..jobs import Phase
from ._common import Profile, _Worktree, get_registry, validate_build_worktree
from .models import JobInfo, job_info, start_message

_Profile = Annotated[Profile, Field(description="Build profile to build with.")]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_build_start",
        description=(
            "Build Netdata with Ninja for a profile. Returns a job_id immediately; poll "
            "netdata_job_status until it finishes (first build can take minutes; incremental "
            "much faster). If the worktree's build dir isn't configured for this profile, the "
            "job configures it first (one job, phased log: configure then build). Profiles: "
            "debug (Debug + internal checks), optimized (RelWithDebInfo). The build goes in "
            "<worktree>/build/ — use a worktree dedicated to LLM runs (no manual build there)."
        ),
    )
    async def netdata_build_start(ctx: Context, worktree: _Worktree, profile: _Profile) -> JobInfo:
        if (err := validate_build_worktree(worktree)) is not None:
            return err

        registry = get_registry(ctx)
        phases: list[Phase] = []
        if buildcfg.needs_configure(worktree, profile):
            phases.append(
                Phase(name="configure", cmd=buildcfg.configure_command(worktree, profile), cwd=worktree)
            )
        phases.append(Phase(name="build", cmd=buildcfg.build_command(worktree), cwd=worktree))

        result = await registry.start(
            kind="build",
            profile=profile,
            key=buildcfg.lock_key(worktree),
            worktree=worktree,
            phases=phases,
            log_path=buildcfg.log_path(worktree),
            lockfile=buildcfg.lock_file(worktree),
        )
        return job_info(result.job, outcome=result.outcome, message=start_message(result, "build"))
