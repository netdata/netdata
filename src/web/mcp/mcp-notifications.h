// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_NOTIFICATIONS_H
#define NETDATA_MCP_NOTIFICATIONS_H

#include "mcp.h"

// Notifications namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_notifications_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id);

#endif // NETDATA_MCP_NOTIFICATIONS_H