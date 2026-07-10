"""Forward a tool call to a running agent's own ``/mcp`` (transport-free: no
build-MCP server imports).

Per call: open a streamable-HTTP MCP client to ``<base_url>/mcp``, initialize,
call the tool, return its textual content. The agent's ``/mcp`` is stateless
(auto ``Mcp-Session-Id``), so a fresh connect per call avoids any session
lifecycle/teardown concerns.

Auth invariant: a per-agent Cloud bearer is always attached as
``Authorization: Bearer``. Agent functions are access-gated (e.g. ``SIGNED_ID``)
even on localhost once the agent is claimed; an anonymous call would 412. The
caller (``tools/agent_mcp.py``) mints the bearer and it is required here — there
is no anonymous path.
"""

from __future__ import annotations

from typing import Any

from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client
from mcp.shared._httpx_utils import create_mcp_http_client
from mcp.shared.exceptions import McpError


def _scrub(text: str, bearer: str) -> str:
    """Redact the bearer from any string returned to the caller (HARD contract)."""
    return text.replace(bearer, "<REDACTED>") if bearer else text


def _content_text(result: Any) -> str:
    """Flatten an MCP CallToolResult's content blocks to text (verbatim forward)."""
    parts: list[str] = []
    for block in getattr(result, "content", None) or []:
        text = getattr(block, "text", None)
        parts.append(text if text is not None else str(block))
    return "\n".join(parts)


async def _call(session: ClientSession, tool: str, arguments: dict[str, Any]) -> str:
    """Call ``tool`` and flatten its content to text. A protocol-level rejection is
    raised as ``McpError`` and handled (and scrubbed) by ``call_agent_tool``."""
    return _content_text(await session.call_tool(tool, arguments))


async def call_agent_tool(base_url: str, tool: str, arguments: dict[str, Any], *, bearer: str) -> str:
    """Call ``tool`` on the agent's MCP at ``<base_url>/mcp``; return its text content.

    ``bearer`` is a minted per-agent Cloud bearer, attached as ``Authorization:
    Bearer`` on the /mcp connection (required — the caller guarantees it). The
    agent's own ``/mcp`` validates arguments and produces the response. Errors
    are forwarded verbatim as text, never raised: a tool-execution failure comes
    back as content, a protocol-level rejection (e.g. a missing required argument)
    surfaces as ``McpError``, and an unreachable agent/handshake failure as any
    other exception — all become a readable, bearer-scrubbed string so the wrapper
    tool returns cleanly instead of crashing.
    """
    url = base_url.rstrip("/") + "/mcp"
    # A pre-configured httpx client is the supported way to attach auth (the
    # streamable transport's own header/auth kwargs are deprecated). Build it via
    # the SDK's own factory so MCP defaults are preserved verbatim — notably the
    # 30s general / 300s SSE-read timeouts — and just add the Authorization header.
    try:
        async with create_mcp_http_client(headers={"Authorization": f"Bearer {bearer}"}) as http_client:
            async with streamable_http_client(url, http_client=http_client) as (read, write, _):
                async with ClientSession(read, write) as session:
                    await session.initialize()
                    return await _call(session, tool, arguments)
    except McpError as exc:  # protocol-level rejection (e.g. a missing required argument)
        return _scrub(f"[agent /mcp error] {exc}", bearer)
    except Exception as exc:  # unreachable agent, handshake failure, etc.
        # An exception repr could in principle include request headers — scrub it.
        return _scrub(f"[could not reach agent /mcp at {url}: {exc!r}]", bearer)
