// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_METRIC_CONTEXTS_H
#define NETDATA_MCP_TOOLS_METRIC_CONTEXTS_H

#include "mcp-tools.h"

// Schema definition function - provides JSON schema for this tool
void mcp_tool_metric_contexts_schema(BUFFER *buffer);

// Execution function - performs the metric contexts query
MCP_RETURN_CODE mcp_tool_metric_contexts_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_TOOLS_METRIC_CONTEXTS_H