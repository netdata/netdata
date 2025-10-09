// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_ADAPTER_WEBSOCKET_H
#define NETDATA_MCP_ADAPTER_WEBSOCKET_H

#include "web/websocket/websocket-internal.h"
#include "web/mcp/mcp.h"

// Initialize the WebSocket adapter for MCP
void mcp_websocket_adapter_initialize(void);

// WebSocket protocol handler callbacks for MCP
void mcp_websocket_on_connect(struct websocket_server_client *wsc);
void mcp_websocket_on_message(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode);
void mcp_websocket_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason);
void mcp_websocket_on_disconnect(struct websocket_server_client *wsc);

// Get and set MCP context from a WebSocket client
MCP_CLIENT *mcp_websocket_get_context(struct websocket_server_client *wsc);
void mcp_websocket_set_context(struct websocket_server_client *wsc, MCP_CLIENT *ctx);

#endif // NETDATA_MCP_ADAPTER_WEBSOCKET_H
