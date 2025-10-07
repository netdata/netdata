// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_HTTP_ADAPTER_H
#define NETDATA_MCP_HTTP_ADAPTER_H

struct rrdhost;
struct web_client;

int mcp_http_handle_request(struct rrdhost *host, struct web_client *w);

#endif // NETDATA_MCP_HTTP_ADAPTER_H
