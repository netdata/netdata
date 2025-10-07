// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_SSE_ADAPTER_H
#define NETDATA_MCP_SSE_ADAPTER_H

#include "web/mcp/mcp.h"

struct rrdhost;
struct web_client;
struct json_object;

int mcp_sse_handle_request(struct rrdhost *host, struct web_client *w);
int mcp_sse_serialize_response(struct web_client *w, MCP_CLIENT *mcpc, struct json_object *root);


#endif // NETDATA_MCP_SSE_ADAPTER_H
