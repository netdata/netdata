// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_WEIGHTS_H
#define NETDATA_MCP_TOOLS_WEIGHTS_H

#include "mcp.h"

// Execute the find_correlated_metrics tool
MCP_RETURN_CODE mcp_tool_find_correlated_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);
void mcp_tool_find_correlated_metrics_schema(BUFFER *buffer);

// Execute the find_anomalous_metrics tool
MCP_RETURN_CODE mcp_tool_find_anomalous_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);
void mcp_tool_find_anomalous_metrics_schema(BUFFER *buffer);

// Execute the find_unstable_metrics tool
MCP_RETURN_CODE mcp_tool_find_unstable_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);
void mcp_tool_find_unstable_metrics_schema(BUFFER *buffer);

#endif // NETDATA_MCP_TOOLS_WEIGHTS_H