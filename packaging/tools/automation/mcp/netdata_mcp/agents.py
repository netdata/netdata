"""Agent registry (transport-free: no MCP imports).

Maps an LLM-supplied ``agent-id`` to its spec ``{worktree, profile, ...}``. The
agent-id is the user-facing handle; the build behind it is the worktree's single
``build/`` (the profile sets its build type). Runtime fields (port, run job) are
attached by the run layer once it launches.

In-memory only (does not survive a server restart).
"""

from __future__ import annotations

from dataclasses import dataclass, field

from . import buildcfg, runtime
from .runtime import OtelConfig


@dataclass
class AgentSpec:
    agent_id: str
    worktree: str
    profile: str
    # otel-plugin tuning applied at the next launch/restart (set via
    # netdata_agent_otel_config). Preserved across idempotent re-declare.
    otel: OtelConfig = field(default_factory=OtelConfig)


class AgentRegistry:
    def __init__(self) -> None:
        self._agents: dict[str, AgentSpec] = {}

    def declare(self, agent_id: str, worktree: str, profile: str) -> AgentSpec:
        """Register (or idempotently update) an agent. Validates id and profile."""
        runtime.sanitize_agent_id(agent_id)
        buildcfg.validate_profile(profile)
        existing = self._agents.get(agent_id)
        if existing is not None:
            existing.worktree = worktree
            existing.profile = profile
            return existing
        spec = AgentSpec(agent_id=agent_id, worktree=worktree, profile=profile)
        self._agents[agent_id] = spec
        return spec

    def get(self, agent_id: str) -> AgentSpec | None:
        return self._agents.get(agent_id)

    def set_otel(self, agent_id: str, otel: OtelConfig) -> AgentSpec | None:
        """Replace the agent's otel config; applied at the next launch/restart.

        Returns the updated spec, or None if the agent isn't declared.
        """
        spec = self._agents.get(agent_id)
        if spec is None:
            return None
        spec.otel = otel
        return spec
