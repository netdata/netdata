"""Register an MCP tool from an explicit (vendored) JSON schema + a generic forwarder.

FastMCP's ``@tool``/``add_tool`` infer the input schema from a Python signature and
offer no way to supply one explicitly. But a ``Tool`` advertises ``parameters`` to
clients (``list_tools`` → ``inputSchema``) while validating/calling through a
*separate* ``fn_metadata``. We exploit that split: ``parameters`` is the vendored
schema the LLM sees, and a permissive arg model forwards whatever the client sends
to a single handler — so we can mirror the agent's own ``/mcp`` tools as native,
typed build-MCP tools without hand-writing 13 signatures.

This leans on SDK internals (``Tool``, ``FuncMetadata``, ``ArgModelBase``, the tool
manager's ``_tools`` map). It is not pinned beyond ``mcp>=1.27.2``; the guard is the
test suite (it registers, lists, and calls a vendored tool), so a breaking SDK
upgrade fails CI rather than ships silently — re-run the tests on upgrade.
"""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import Any

from mcp.server.fastmcp import Context, FastMCP
from mcp.server.fastmcp.tools.base import Tool
from mcp.server.fastmcp.utilities.func_metadata import ArgModelBase, FuncMetadata
from pydantic import ConfigDict

# handler: (ctx, agent_id, arguments) -> result; arguments excludes agent_id.
Forwarder = Callable[[Context, str, dict[str, Any]], Awaitable[Any]]


class _PassthroughArgs(ArgModelBase):
    """Accept any client arguments and surface them for forwarding.

    The vendored ``parameters`` schema guides the LLM; the agent's own ``/mcp`` does
    the real validation, so this model intentionally validates nothing and just
    passes the fields through (FastMCP only forwards declared fields by default, so
    we override the dump to return the ``extra`` ones).
    """

    model_config = ConfigDict(extra="allow")

    def model_dump_one_level(self) -> dict[str, Any]:
        return dict(self.__pydantic_extra__ or {})


def register_forwarding_tool(
    mcp: FastMCP,
    *,
    name: str,
    description: str,
    input_schema: dict[str, Any],
    forward: Forwarder,
) -> None:
    """Register ``name`` advertising ``input_schema``; calls go to ``forward``.

    ``input_schema`` is the vendored JSON schema (it must include ``agent_id``).
    The injected ``agent_id`` is split out and the remaining arguments are passed to
    ``forward(ctx, agent_id, arguments)``.
    """

    async def _fn(ctx: Context, **kwargs: Any) -> Any:
        agent_id = kwargs.pop("agent_id", None)
        return await forward(ctx, agent_id, kwargs)

    mcp._tool_manager._tools[name] = Tool(
        fn=_fn,
        name=name,
        description=description,
        parameters=input_schema,
        fn_metadata=FuncMetadata(arg_model=_PassthroughArgs),
        is_async=True,
        context_kwarg="ctx",
    )
