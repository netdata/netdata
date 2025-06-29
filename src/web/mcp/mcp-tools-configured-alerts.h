// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_CONFIGURED_ALERTS_H
#define NETDATA_MCP_TOOLS_CONFIGURED_ALERTS_H

#include "mcp-tools.h"

#define MCP_TOOL_LIST_CONFIGURED_ALERTS "list_configured_alerts"

// Schema generator function
void mcp_tool_list_configured_alerts_schema(BUFFER *buffer);

// Execution function
MCP_RETURN_CODE mcp_tool_list_configured_alerts_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

#endif //NETDATA_MCP_TOOLS_CONFIGURED_ALERTS_H