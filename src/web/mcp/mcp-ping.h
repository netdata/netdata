// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_PING_H
#define NETDATA_MCP_PING_H

#include "libnetdata/libnetdata.h"
#include <json-c/json.h>
#include "mcp.h"

/**
 * Handle ping requests from a client
 * 
 * @param mcpc The MCP client context
 * @param params The JSON params object (should be empty for ping)
 * @param id The request ID
 * @return MCP_RETURN_CODE - MCP_RC_OK if successful
 */
MCP_RETURN_CODE mcp_method_ping(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

#endif // NETDATA_MCP_PING_H