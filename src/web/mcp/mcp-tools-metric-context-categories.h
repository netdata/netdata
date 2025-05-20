// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_METRIC_CONTEXT_CATEGORIES_H
#define NETDATA_MCP_TOOLS_METRIC_CONTEXT_CATEGORIES_H

#include "mcp-tools.h"

// Schema definition function - provides JSON schema for this tool
void mcp_tool_metric_context_categories_schema(BUFFER *buffer);

// Execution function - performs the metric context categories query
MCP_RETURN_CODE mcp_tool_metric_context_categories_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_TOOLS_METRIC_CONTEXT_CATEGORIES_H