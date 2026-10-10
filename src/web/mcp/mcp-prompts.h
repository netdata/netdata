// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_PROMPTS_H
#define NETDATA_MCP_PROMPTS_H

#include "mcp.h"

#define MCP_PROMPT_TROUBLESHOOT_ALERT "troubleshoot_alert"
#define MCP_PROMPT_EXPLAIN_ANOMALY "explain_anomaly"

// Maximum diagnostic context payload size (16 KiB)
#define MCP_PROMPT_CONTEXT_MAX_BYTES (16 * 1024)

// Prompts namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_prompts_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id);

// Unit test for MCP prompts subsystem
int mcp_prompts_unittest(void);

#endif // NETDATA_MCP_PROMPTS_H
