// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_INITIALIZE_H
#define NETDATA_MCP_INITIALIZE_H

#include "mcp.h"

// Initialize method handler (transport-agnostic)
MCP_RETURN_CODE mcp_method_initialize(MCP_CLIENT *mcpc, struct json_object *params, uint64_t id);

#endif // NETDATA_MCP_INITIALIZE_H