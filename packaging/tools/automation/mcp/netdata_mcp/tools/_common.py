"""Shared helpers for tool modules."""

from __future__ import annotations

from typing import Annotated, Literal

from mcp.server.fastmcp import Context
from pydantic import Field

from .. import buildcfg
from ..agents import AgentRegistry
from ..jobs import JobRegistry
from ..run import RunRegistry
from ..streams import StreamRegistry
from .models import JobInfo, input_error

# Kept in sync with profiles.PROFILES (enforced by tests/test_profiles.py) so the
# tool schema constrains the profile argument to valid choices.
Profile = Literal["debug", "optimized"]

_Worktree = Annotated[str, Field(description="Absolute path to the Netdata worktree (must contain CMakeLists.txt).")]


def get_registry(ctx: Context) -> JobRegistry:
    return ctx.request_context.lifespan_context.registry


def get_agents(ctx: Context) -> AgentRegistry:
    return ctx.request_context.lifespan_context.agents


def get_runs(ctx: Context) -> RunRegistry:
    return ctx.request_context.lifespan_context.runs


def get_streams(ctx: Context) -> StreamRegistry:
    return ctx.request_context.lifespan_context.streams


def validate_build_worktree(worktree: str) -> JobInfo | None:
    """Shared build-tool guard: confirm it's a worktree and claim its build dir.

    Returns an error ``JobInfo`` to return to the client, or None if all good.
    """
    if not buildcfg.is_worktree(worktree):
        return input_error(f"Not a Netdata worktree (no CMakeLists.txt): {worktree}")
    try:
        buildcfg.claim_build_dir(worktree)
    except (buildcfg.BuildDirNotOwned, OSError) as exc:
        return input_error(str(exc))
    return None
