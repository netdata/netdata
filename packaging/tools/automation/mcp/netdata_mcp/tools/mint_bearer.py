"""Mint a Netdata Cloud per-agent bearer and RETURN it, for the Playwright
(browser) MCP.

The dashboard renders SIGNED_ID-gated agent functions (otel-logs, otel-files,
systemd-journal) only when the browser's requests carry an
``Authorization: Bearer <token>`` with a signed identity; a headless browser
hitting the agent anonymously gets HTTP 412. The netdata-build MCP and the
Playwright MCP cannot talk directly, so this tool returns the minted bearer to
the LLM, which injects it into the page (see the tool description for the exact
``browser_run_code_unsafe`` snippet).

Token-safety exception (deliberate, localhost-dev only): unlike ``otel_logs`` /
``otel_files`` — which mint a bearer internally and NEVER expose it — this tool
returns the minted per-agent bearer in its result. The netdata-build MCP is a
localhost developer tool whose only off-box hop is agent <-> Netdata Cloud, so
exposing the per-agent bearer to the local LLM is acceptable. The Cloud *account*
token (``NETDATA_CLOUD_TOKEN``) is still never returned or logged — only the
short-lived per-agent bearer minted from it.
"""

from __future__ import annotations

from typing import Annotated

from mcp.server.fastmcp import Context, FastMCP
from pydantic import BaseModel, Field

from .. import bearer
from ._common import get_runs

_AgentId = Annotated[str, Field(description="A ready, claimed agent (from netdata_agent_declare + netdata_run_start).")]
_Timeout = Annotated[int, Field(description="Seconds to wait for the mint (agent /api/v3/info probe + Cloud round-trip).", ge=1, le=120)]


class MintBearerResult(BaseModel):
    """Result of a bearer mint. ``bearer`` is the per-agent Cloud token to
    inject into the browser as ``Authorization: Bearer <bearer>`` (None on
    error)."""

    agent_id: str
    bearer: str | None = Field(
        default=None,
        description="The minted per-agent Cloud bearer. Inject into the page as 'Authorization: Bearer <bearer>' (see the tool description).",
    )
    error: str | None = None
    message: str = ""


async def _mint_bearer(ctx: Context, agent_id: str, timeout: int) -> MintBearerResult:
    """Tool body, split out so it can be unit-tested with a fake ctx (mirrors
    agent_mcp._forward_to_agent)."""
    run = get_runs(ctx).get(agent_id)
    if run is None or run.state != "ready" or not run.port:
        state = run.state if run is not None else "unknown"
        return MintBearerResult(
            agent_id=agent_id,
            error=f"Agent {agent_id!r} is not ready (state={state}). "
            "Start it with netdata_run_start and poll netdata_run_status until 'ready'.",
        )
    token, err = await bearer.resolve_bearer(run.port, timeout=timeout)
    if token is None:
        # resolve_bearer already names the missing token; only add that hint when it didn't.
        hint = "Ensure the agent is claimed + cloud_connected (check netdata_run_status)."
        if not (err and "NETDATA_CLOUD_TOKEN" in err):
            hint = "Ensure NETDATA_CLOUD_TOKEN is set and " + hint[len("Ensure ") :]
        return MintBearerResult(agent_id=agent_id, error=f"could not mint a bearer ({err}). {hint}")
    return MintBearerResult(
        agent_id=agent_id,
        bearer=token,
        message="ok — inject as Authorization: Bearer <bearer> via Playwright "
        "browser_run_code_unsafe (see the tool description), then re-trigger the request.",
    )


def register(mcp: FastMCP) -> None:
    @mcp.tool(
        name="netdata_agent_mint_bearer",
        description=(
            "Mint a Netdata Cloud per-agent bearer and RETURN it, for use with the "
            "Playwright (browser) MCP. Use this to view SIGNED_ID-gated agent functions "
            "— otel-logs, otel-files, systemd-journal — in the Agent dashboard UI: a "
            "headless browser hitting the agent anonymously gets HTTP 412 on those "
            "functions, so inject this bearer to authorize them. WORKFLOW: "
            "(1) call this to get `bearer`; "
            "(2) via the Playwright MCP's browser_run_code_unsafe, run "
            "(substitute the returned bearer for BEARER): "
            "async (page) => { await page.setExtraHTTPHeaders({ Authorization: 'Bearer ' + 'BEARER' }); return 'auth-set'; } ; "
            "(3) re-trigger the request — navigate within the dashboard (e.g. switch "
            "tabs) or reload — so the gated call re-fires with the header, returns 200, "
            "and renders. The header persists on the page across navigations/reloads "
            "until the page closes. Requires NETDATA_CLOUD_TOKEN in the server env + a "
            "claimed, cloud_connected agent (else returns an error). Bearers last ~3h; "
            "re-mint if a long session starts 412-ing again. Localhost-dev tool — the "
            "per-agent bearer is intentionally exposed here."
        ),
    )
    async def netdata_agent_mint_bearer(
        ctx: Context,
        agent_id: _AgentId,
        timeout: _Timeout = 30,
    ) -> MintBearerResult:
        return await _mint_bearer(ctx, agent_id, timeout)
