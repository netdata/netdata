"""MCP tool registration modules.

Each module exposes ``register(mcp)`` that attaches its tools to the shared
``FastMCP`` instance.  Adding a capability domain = a new module + one
``register()`` call in :mod:`netdata_mcp.server`.
"""
