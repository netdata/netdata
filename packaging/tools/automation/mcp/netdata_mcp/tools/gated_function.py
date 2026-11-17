"""Shared call path for the OTel plugin's access-gated Functions.

``otel-logs``, ``otel-traces`` and ``topology:tracing`` declare ``SIGNED_ID``,
so a local agent rejects an anonymous call with HTTP 412 on every transport;
there is no localhost bypass. When ``NETDATA_CLOUD_TOKEN`` is in the server env
this mints and sends a Cloud bearer for the (claimed) agent (see ``bearer.py``);
without it the call goes out anonymously and the 412 hint explains the gate.
"""

from __future__ import annotations

from typing import Annotated, Any

from mcp.server.fastmcp import Context
from pydantic import BaseModel, Field

from .. import agentfn, bearer
from ._common import get_runs

# Ceiling for the bearer mint's two HTTP hops (loopback /api/v3/info + the Cloud
# round-trip), kept below the call timeout so it cannot starve the call itself.
_MINT_TIMEOUT = 20

AgentId = Annotated[str, Field(description="A ready agent (from netdata_agent_declare + netdata_run_start).")]
Timeout = Annotated[int, Field(description="Seconds to wait for the function.", ge=1, le=600)]


class FunctionResult(BaseModel):
    """One Function call. ``response`` is the parsed JSON body, success or error,
    so callers can assert on any field."""

    agent_id: str
    function: str
    endpoint: str | None = None
    http_status: int | None = None
    request: dict[str, Any] | None = Field(default=None, description="The payload POSTed to the function.")
    response: dict[str, Any] | None = Field(default=None, description="Parsed function response.")
    error: str | None = None
    message: str = ""


def classify_status(function: str, status: int | None, data: Any = None) -> tuple[str | None, str]:
    """Map an HTTP status to ``(error, message)``. Only 2xx is success, so a
    caller checking ``error is None`` cannot mistake a 4xx/5xx for data. The
    body's ``errorMessage`` (the Functions error shape) is appended when present."""
    if status is not None and 200 <= status < 300:
        return None, "ok"
    if status == 412:
        msg = (
            f"{function} returned 412 — access-gated (SIGNED_ID). Set NETDATA_CLOUD_TOKEN "
            "in the server env so this tool mints a bearer, and ensure the agent is "
            "claimed + cloud_connected."
        )
        return msg, msg
    msg = f"{function} returned HTTP {status}"
    if isinstance(data, dict) and isinstance(data.get("errorMessage"), str):
        msg = f"{msg}: {data['errorMessage']}"
    return msg, msg


async def call_gated_function(
    ctx: Context, agent_id: str, function: str, payload: dict[str, Any], timeout: int
) -> FunctionResult:
    """Resolve ``agent_id`` to its ready agent, mint a bearer when a Cloud token
    is configured, POST ``payload`` to ``function``, and classify the answer.
    Never raises."""
    run = get_runs(ctx).get(agent_id)
    if run is None or run.state != "ready" or not run.port:
        state = run.state if run is not None else "unknown"
        return FunctionResult(
            agent_id=agent_id, function=function, request=payload,
            error=f"Agent {agent_id!r} is not ready (state={state}). "
            "Start it with netdata_run_start and poll netdata_run_status until 'ready'.",
        )
    base = f"http://127.0.0.1:{run.port}"
    endpoint = agentfn.function_url(base, function, timeout)
    # A mint failure is hard: an anonymous retry would only 412.
    auth: str | None = None
    if bearer.cloud_token():
        auth, berr = await bearer.resolve_bearer(run.port, timeout=min(timeout, _MINT_TIMEOUT))
        if auth is None:
            return FunctionResult(
                agent_id=agent_id, function=function, endpoint=endpoint, request=payload,
                error=f"could not obtain a Netdata Cloud bearer ({berr}). {function} requires "
                "a signed-in identity; ensure the agent is claimed and cloud_connected "
                "(netdata_run_status).",
            )
    status, data, err = await agentfn.call_function(base, function, payload, timeout=timeout, bearer=auth)
    if err is not None:
        return FunctionResult(
            agent_id=agent_id, function=function, endpoint=endpoint, http_status=status,
            request=payload, error=err,
        )
    result_error, message = classify_status(function, status, data)
    return FunctionResult(
        agent_id=agent_id, function=function, endpoint=endpoint, http_status=status, request=payload,
        response=data if isinstance(data, dict) else {"value": data},
        error=result_error, message=message,
    )
