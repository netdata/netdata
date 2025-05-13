// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_H
#define NETDATA_MCP_TOOLS_H

#include "mcp.h"

// Tools namespace method dispatcher (transport-agnostic)
int mcp_tools_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id);

#endif // NETDATA_MCP_TOOLS_H