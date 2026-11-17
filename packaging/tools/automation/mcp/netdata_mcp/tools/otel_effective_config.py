"""Otel domain: read back the config the otel plugin actually applied.

At startup the plugin resolves stock -> user otel.yaml -> env, validates, and
logs one ``effective config: {json}`` line (``otel-plugin/src/config/mod.rs``,
``log_config``), with ``remote_storage.uri`` redacted to its scheme. This tool
finds that line for the agent's current run: in the journal on journald hosts
(where the agent routes its logs, see ``runtime._default_conf``), else in the
run's captured output. Journal entries carry no per-agent field, so the run's
supervisor is identified by its ``netdata environment`` startup line, logged
before the config is loaded and naming the agent's own config dir; only that
process's lines are used, whether or not it is still alive. No line means the plugin refused its config or has not
started; the run's config-related log lines are returned instead.
"""

from __future__ import annotations

import json
import re
from typing import Any, Literal

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, Field

from .. import journal
from ._common import get_runs
from .gated_function import AgentId

_MARKER = "effective config: "
# journalctl short-iso: "<time> <host> otel-plugin[<pid>]: <message>"
_JOURNAL_PID_RE = re.compile(r"\sotel-plugin\[(\d+)\]: ")
_CONTEXT_LINES = 20


class OtelEffectiveConfig(BaseModel):
    agent_id: str
    source: Literal["journal", "run_logs"] | None = None
    effective: dict[str, Any] | None = Field(default=None, description="The resolved plugin config, as logged.")
    config_log: str | None = Field(
        default=None, description="The run's config-related log lines, when no effective config was found."
    )
    error: str | None = None
    message: str = ""


def parse_effective(lines: list[str]) -> dict[str, Any] | None:
    """The last ``effective config`` JSON among ``lines``, or None."""
    for line in reversed(lines):
        _, sep, rest = line.partition(_MARKER)
        if not sep:
            continue
        try:
            value = json.loads(rest)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            return value
    return None


def scope_to_run(lines: list[str], user_config_dir: str) -> list[str]:
    """The journal ``lines`` of the supervisor(s) whose ``netdata environment``
    line names ``user_config_dir``; empty when no such line is present."""
    needle = f'"user_config_dir":{json.dumps(user_config_dir)}'
    pids = set()
    for line in lines:
        m = _JOURNAL_PID_RE.search(line)
        if m and "netdata environment: " in line and needle in line:
            pids.add(m.group(1))
    return [line for line in lines if (m := _JOURNAL_PID_RE.search(line)) and m.group(1) in pids]


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_otel_effective_config",
        description=(
            "Return the config the otel plugin actually applied in the agent's current run "
            "(stock otel.yaml + the user file from netdata_agent_otel_config + env, after "
            "validation and the plugin's own corrections), parsed from its startup log line. "
            "remote_storage.uri appears redacted to its scheme. If none is found (the plugin "
            "refused the config, or it has not started yet), config_log holds the run's "
            "config-related log lines, refusal included."
        ),
    )
    async def netdata_agent_otel_effective_config(ctx: Context, agent_id: AgentId) -> OtelEffectiveConfig:
        run = get_runs(ctx).get(agent_id)
        if run is None:
            return OtelEffectiveConfig(
                agent_id=agent_id,
                error=f"No run for {agent_id!r}. Start it with netdata_run_start.",
            )
        source: Literal["journal", "run_logs"]
        if journal.journald_socket_present():
            source = "journal"
            # "config" also matches the environment line (user_config_dir).
            text = await journal.read_logs(
                identifier=journal.syslog_identifier("supervisor"), pid=None, lines=500,
                grep="config", priority=None, since=run.created_wall,
            )
            lines = scope_to_run(text.splitlines(), str(run.run_dir / "etc"))
        else:
            source = "run_logs"
            lines = [line for line in run.buffer.read(0).text.splitlines() if "config" in line]
        effective = parse_effective(lines)
        if effective is not None:
            return OtelEffectiveConfig(agent_id=agent_id, source=source, effective=effective, message="ok")
        return OtelEffectiveConfig(
            agent_id=agent_id, source=source,
            config_log="\n".join(lines[-_CONTEXT_LINES:]),
            error=f"no effective config logged for {agent_id!r} (state={run.state}): the plugin "
            "refused its config or has not started yet; see config_log.",
        )
