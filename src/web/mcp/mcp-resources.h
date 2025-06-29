// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_RESOURCES_H
#define NETDATA_MCP_RESOURCES_H

#include "mcp.h"

// Resources namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_resources_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_RESOURCES_H