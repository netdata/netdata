# MCP Logs Tools Implementation Plan

## Overview

Currently, logs functions (`has_history=true`) are processed as regular tables by the `execute_function` tool, which completely misses the powerful facets functionality that Netdata's logs system provides. We need to split logs into dedicated tools that expose the full power of facets analysis to LLMs.

## Progressive Logs Discovery Workflow

The logs system should provide 4 specialized tools that enable LLMs to progressively discover and analyze log data:

### 1. `list_logs_sources`
- **Purpose**: Discovery - "What logs are available?"
- **Returns**: Nodes with logs capabilities, available sources per node
- **Example**: `systemd-journal` on Linux nodes, `windows-events` on Windows nodes
- **Parameters**: Maybe filter by node patterns?

### 2. `list_logs_fields` 
- **Purpose**: Schema discovery - "What fields exist in this source?"
- **Parameters**: node, source(s)
- **Returns**: All available fields/columns for the selected sources
- **Example**: For systemd-journal: `MESSAGE`, `_PID`, `_COMM`, `PRIORITY`, etc.

### 3. `list_logs_fields_values`
- **Purpose**: Facets analysis - "What values can these fields have and how common are they?"
- **Parameters**: node, source(s), field(s), optional conditions, timeframe
- **Returns**: Field values with counts - **this is the core facets functionality!**
- **Example**: `PRIORITY` field → `{0: 1234, 3: 5678, 6: 2345}` (counts per priority level)

### 4. `query_logs`
- **Purpose**: Actual log retrieval with all the power of the current system
- **Parameters**: node, source(s), conditions, timeframe, pagination, etc.
- **Returns**: Log entries matching criteria

## Implementation Strategy

This maps nicely to the existing facets infrastructure:

- **Tools 1-2**: Use the `info=true` calls to logs functions to get metadata
- **Tool 3**: Call logs functions with specific facets parameters, return just the facets counts
- **Tool 4**: Current full query capability

The beauty is that **tool 3 gives LLMs the exact capability described** - counting field values with conditions across timeframes!

## Benefits of This Approach

1. **Clean separation** - Logs tools optimized for time-series log analysis
2. **Progressive discovery** - LLMs can build up knowledge step by step
3. **Exposes facets power** - Tool 3 provides the counting/analysis capability that was missing
4. **No duplication** - Can share common code for function execution and response processing
5. **Better LLM experience** - Each tool focused on a specific discovery/analysis task

## Current Status

### Logs Tools Implementation
- [x] Design the 4-tool architecture
- [ ] Implement `list_logs_sources`
- [ ] Implement `list_logs_fields`
- [ ] Implement `list_logs_fields_values` (the key facets tool)
- [ ] Implement `query_logs`
- [ ] Update `execute_function` to exclude logs functions
- [ ] Test the complete workflow

### MCP Transport Refactoring
- [ ] Isolate JSONRPC into websocket-adapter
- [ ] Support multi-buffer responses from all tools
- [ ] Create HTTP adapter for MCP with streamable HTTP support (new MCP standard)

## Key Implementation Notes

### Logs Tools
- Tool 3 (`list_logs_fields_values`) is the most important as it exposes the facets counting capability
- All tools should reuse existing logs function infrastructure via rrdfunctions
- Schema should be designed to be LLM-friendly with clear parameter descriptions
- Consider timeout handling for large facets analysis operations

### Transport Refactoring
- Currently JSONRPC is embedded in the websocket adapter
- Need to isolate JSONRPC to support multiple transport protocols
- Multi-buffer responses will enable streaming for long-running operations
- HTTP adapter with streamable HTTP will provide broader compatibility with MCP clients
- This enables Netdata MCP server to work with the new streamable HTTP standard that MCP supports