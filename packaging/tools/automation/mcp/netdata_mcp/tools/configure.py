"""Configure domain: start a cmake-configure job for a worktree."""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import Field

from .. import buildcfg
from ..jobs import Phase
from ._common import Profile, _Worktree, get_registry, validate_build_worktree
from .models import JobInfo, job_info, start_message

_Profile = Annotated[Profile, Field(description="Build profile to configure for.")]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_configure_start",
        description=(
            "Configure (cmake) a Netdata worktree for a build profile. Returns a job_id "
            "immediately; poll netdata_job_status until it finishes. Profiles: debug "
            "(Debug + internal checks) and optimized (RelWithDebInfo); both use a curated "
            "plugin set (common features on, heavy/rare off). Configures <worktree>/build/ "
            "— use a worktree dedicated to LLM runs. You usually don't need this directly — "
            "netdata_build_start configures first if needed."
        ),
    )
    async def netdata_configure_start(ctx: Context, worktree: _Worktree, profile: _Profile) -> JobInfo:
        if (err := validate_build_worktree(worktree)) is not None:
            return err

        registry = get_registry(ctx)
        phase = Phase(
            name="configure",
            cmd=buildcfg.configure_command(worktree, profile),
            cwd=worktree,
        )
        result = await registry.start(
            kind="configure",
            profile=profile,
            key=buildcfg.lock_key(worktree),
            worktree=worktree,
            phases=[phase],
            log_path=buildcfg.log_path(worktree),
            lockfile=buildcfg.lock_file(worktree),
        )
        return job_info(result.job, outcome=result.outcome, message=start_message(result, "configure"))
