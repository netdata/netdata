// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_LOGS_H
#define NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_LOGS_H

#include "mcp-tools-execute-function-internal.h"

// Process logs result from function execution
MCP_RETURN_CODE mcp_functions_process_logs(MCP_FUNCTION_DATA *data, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_TOOLS_EXECUTE_FUNCTION_LOGS_H