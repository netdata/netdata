// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_H
#define NETDATA_MCP_TOOLS_H

#include "mcp.h"

// Tools namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_tools_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id);

// Standardized schema parameter helpers
void mcp_schema_params_add_time_window(BUFFER *buffer, const char *output_type, bool is_data_query);
void mcp_schema_params_add_cardinality_limit(BUFFER *buffer, const char *output_type, bool is_data_query);

#endif // NETDATA_MCP_TOOLS_H