// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_ALERT_TRANSITIONS_H
#define NETDATA_MCP_TOOLS_ALERT_TRANSITIONS_H

#include "mcp.h"

// Function declarations for alert transitions tool
void mcp_tool_list_alert_transitions_schema(BUFFER *buffer);
MCP_RETURN_CODE mcp_tool_list_alert_transitions_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

#endif //NETDATA_MCP_TOOLS_ALERT_TRANSITIONS_H