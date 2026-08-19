// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_H
#define NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_H

#include "mcp-tools.h"

// Schema definition function - provides JSON schema for this tool
void mcp_tool_execute_function_schema(BUFFER *buffer);

// Execution function - performs the function execution
MCP_RETURN_CODE mcp_tool_execute_function_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

// Regression test: anonymous execute_function must not disclose protected function
// metadata (GHSA-6628-vxm3-4g8g). Requires a prepared RRD (localhost). Returns the
// number of failures (0 = pass).
int mcp_execute_function_access_unittest(void);

#endif // NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_H