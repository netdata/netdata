// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-websocket.h"
#include "web/websocket/websocket-internal.h"

// Store the MCP context in the WebSocket client's data field
void mcp_websocket_set_context(struct websocket_server_client *wsc, MCP_CLIENT *ctx) {
    if (!wsc) return;
    wsc->user_data = ctx;
}

// Get the MCP context from a WebSocket client
MCP_CLIENT *mcp_websocket_get_context(struct websocket_server_client *wsc) {
    if (!wsc) return NULL;
    return (MCP_CLIENT *)wsc->user_data;
}

// WebSocket buffer sender function for the MCP adapter
int mcp_websocket_send_buffer(struct websocket_server_client *wsc, BUFFER *buffer) {
    if (!wsc || !buffer) return -1;
    
    const char *text = buffer_tostring(buffer);
    if (!text || !*text) return -1;
    
    return websocket_protocol_send_text(wsc, text);
}

// Create a response context for a WebSocket client
static MCP_CLIENT *mcp_websocket_create_context(struct websocket_server_client *wsc) {
    if (!wsc) return NULL;

    MCP_CLIENT *ctx = mcp_create_client(MCP_TRANSPORT_WEBSOCKET, wsc);
    mcp_websocket_set_context(wsc, ctx);
    
    return ctx;
}

// WebSocket connection handler for MCP
void mcp_websocket_on_connect(struct websocket_server_client *wsc) {
    if (!wsc) return;
    
    // Create the MCP context
    MCP_CLIENT *ctx = mcp_websocket_create_context(wsc);
    if (!ctx) {
        websocket_protocol_send_close(wsc, WS_CLOSE_INTERNAL_ERROR, "Failed to create MCP context");
        return;
    }
    
    websocket_debug(wsc, "MCP client connected");
}

// WebSocket message handler for MCP - receives message and routes to MCP
void mcp_websocket_on_message(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode) {
    if (!wsc || !message || length == 0) return;
    
    // Only handle text messages
    if (opcode != WS_OPCODE_TEXT) {
        websocket_debug(wsc, "Ignoring binary message");
        return;
    }
    
    // Get the MCP context
    MCP_CLIENT *ctx = mcp_websocket_get_context(wsc);
    if (!ctx) {
        websocket_debug(wsc, "MCP context not found");
        websocket_protocol_send_close(wsc, WS_CLOSE_INTERNAL_ERROR, "MCP context not found");
        return;
    }
    
    // Parse the JSON-RPC request
    struct json_object *request = NULL;
    enum json_tokener_error jerr = json_tokener_success;
    request = json_tokener_parse_verbose(message, &jerr);
    
    if (!request || jerr != json_tokener_success) {
        websocket_debug(wsc, "Failed to parse JSON-RPC request: %s", json_tokener_error_desc(jerr));
        CLEAN_BUFFER *b = buffer_create(0, NULL);
        mcp_jsonrpc_error(b, NULL, 0, -32700);
        mcp_websocket_send_buffer(wsc, b);
        return;
    }
    
    // Pass the request to the MCP handler
    mcp_handle_request(ctx, request);
    
    // Free the request object
    json_object_put(request);
}

// WebSocket close handler for MCP
void mcp_websocket_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason) {
    if (!wsc) return;
    
    websocket_debug(wsc, "MCP client closing (code: %d, reason: %s)", code, reason ? reason : "none");
    
    // Clean up the MCP context
    MCP_CLIENT *ctx = mcp_websocket_get_context(wsc);
    if (ctx) {
        mcp_free_client(ctx);
        mcp_websocket_set_context(wsc, NULL);
    }
}

// WebSocket disconnect handler for MCP
void mcp_websocket_on_disconnect(struct websocket_server_client *wsc) {
    if (!wsc) return;
    
    websocket_debug(wsc, "MCP client disconnected");
    
    // Clean up the MCP context
    MCP_CLIENT *ctx = mcp_websocket_get_context(wsc);
    if (ctx) {
        mcp_free_client(ctx);
        mcp_websocket_set_context(wsc, NULL);
    }
}

// Register WebSocket callbacks for MCP
void mcp_websocket_adapter_initialize(void) {
    // Initialize the MCP subsystem
    mcp_initialize_subsystem();
    
    netdata_log_info("MCP WebSocket adapter initialized");
}