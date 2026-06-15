"""Forward a tool call to a running agent's own ``/mcp`` (transport-free: no
build-MCP server imports).

Per-call (D3): open a streamable-HTTP MCP client to ``<base_url>/mcp``, initialize,
call the tool, return its textual content. The agent's ``/mcp`` is stateless
(auto ``Mcp-Session-Id``), so a fresh connect per call avoids any session
lifecycle/teardown concerns. No auth is needed on a localhost agent.
"""

from __future__ import annotations

from typing import Any

from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client
from mcp.shared.exceptions import McpError


def _content_text(result: Any) -> str:
    """Flatten an MCP CallToolResult's content blocks to text (verbatim forward)."""
    parts: list[str] = []
    for block in getattr(result, "content", None) or []:
        text = getattr(block, "text", None)
        parts.append(text if text is not None else str(block))
    return "\n".join(parts)


async def _call(session: ClientSession, tool: str, arguments: dict[str, Any]) -> str:
    try:
        result = await session.call_tool(tool, arguments)
    except McpError as exc:
        # protocol-level rejection (e.g. a missing required argument) — forward it
        return f"[agent /mcp error] {exc}"
    return _content_text(result)


async def call_agent_tool(base_url: str, tool: str, arguments: dict[str, Any]) -> str:
    """Call ``tool`` on the agent's MCP at ``<base_url>/mcp``; return its text content.

    The agent's own ``/mcp`` validates arguments and produces the response. Errors
    are forwarded verbatim as text, never raised: a tool-execution failure comes
    back as content, while a protocol-level rejection (e.g. a missing required
    argument) is raised by the client as ``McpError`` — both become a readable
    string so the wrapper tool returns cleanly instead of crashing.
    """
    url = base_url.rstrip("/") + "/mcp"
    try:
        async with streamable_http_client(url) as (read, write, _):
            async with ClientSession(read, write) as session:
                await session.initialize()
                return await _call(session, tool, arguments)
    except Exception as exc:  # unreachable agent, handshake failure, etc.
        return f"[could not reach agent /mcp at {url}: {exc!r}]"
