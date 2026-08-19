#!/usr/bin/env python3
"""Refresh netdata_mcp/agent_tools.py from a running agent's /mcp tool surface.

How the vendored schemas are captured (and how to refresh them):
  1. Start an agent via the build-MCP: netdata_agent_declare + netdata_run_start,
     then netdata_run_status until 'ready' (gives its url).
  2. Run:  python3 scripts/snapshot_agent_tools.py <url>/mcp
It connects to the agent's /mcp, lists its tools, and rewrites agent_tools.py with
each tool's name -> {description, inputSchema}.
"""

import json
import sys
from pathlib import Path

import anyio
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client

OUT = Path(__file__).resolve().parent.parent / "netdata_mcp" / "agent_tools.py"
HEADER = '''"""Vendored snapshot of the Netdata Agent's /mcp tool surface.

Captured from a live agent's tools/list (see scripts/snapshot_agent_tools.py).
These are the agent's OWN tool name -> {description, inputSchema}; the build-MCP
wrapper re-exposes each as `netdata_agent_<name>` with an injected `agent_id` and
forwards to the agent's /mcp. Drift from the live agent is accepted (a pinned
harness surface); refresh by re-running the snapshot script.
"""

from __future__ import annotations

from typing import Any

AGENT_TOOLS: dict[str, dict[str, Any]] = '''


async def main(url: str) -> None:
    async with streamablehttp_client(url) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            listed = await session.list_tools()
            tools = {
                t.name: {"description": t.description or "", "inputSchema": t.inputSchema}
                for t in sorted(listed.tools, key=lambda t: t.name)
            }
    OUT.write_text(HEADER + json.dumps(tools, indent=4) + "\n", encoding="utf-8")
    print(f"wrote {len(tools)} tools -> {OUT}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: snapshot_agent_tools.py <agent /mcp url>")
    anyio.run(main, sys.argv[1])
