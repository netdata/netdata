// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-websocket.h"
#include "web/websocket/websocket-internal.h"
#include "web/mcp/mcp-jsonrpc.h"

#include <string.h>

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

static void mcp_websocket_release_context(struct websocket_server_client *wsc) {
    MCP_CLIENT *ctx = mcp_websocket_get_context(wsc);
    if (!ctx)
        return;

    // Detach before freeing so repeated lifecycle callbacks never observe a stale pointer.
    mcp_websocket_set_context(wsc, NULL);
    mcp_free_client(ctx);
}

// Create a response context for a WebSocket client
static MCP_CLIENT *mcp_websocket_create_context(struct websocket_server_client *wsc) {
    if (!wsc) return NULL;

    MCP_CLIENT *ctx = mcp_create_client(MCP_TRANSPORT_WEBSOCKET, wsc);
    if (ctx) {
        // Set pointer to the websocket client's user_auth
        ctx->user_auth = &wsc->user_auth;
    }
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

static void mcp_websocket_send_payload(struct websocket_server_client *wsc, BUFFER *payload) {
    if (!wsc || !payload)
        return;

    const char *text = buffer_tostring(payload);
    if (!text)
        return;

    netdata_log_debug(D_MCP, "SND: %s", text);
    websocket_protocol_send_text(wsc, text);
}


// WebSocket message handler for MCP - receives message and routes to MCP
void mcp_websocket_on_message(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode) {
    if (!wsc || !message || length == 0)
        return;
    
    // Only handle text messages
    if (opcode != WS_OPCODE_TEXT) {
        websocket_error(wsc, "Ignoring binary message - mcp supports only TEXT messages");
        return;
    }

    // Log the raw incoming message
    netdata_log_debug(D_MCP, "RCV: %s", message);
    
    // Silently ignore standalone "PING" messages (legacy MCP client behavior)
    if (length == 4 && strncmp(message, "PING", 4) == 0) {
        websocket_debug(wsc, "Ignoring legacy PING message");
        return;
    }
    
    // Get the MCP context
    MCP_CLIENT *mcpc = mcp_websocket_get_context(wsc);
    if (!mcpc) {
        websocket_error(wsc, "MCP context not found");
        return;
    }
    
    // Parse the JSON-RPC request
    struct json_object *request = NULL;
    enum json_tokener_error jerr = json_tokener_success;
    request = mcp_jsonrpc_parse_request(message, length, &jerr);

    if (jerr != json_tokener_success) {
        websocket_error(wsc, "Failed to parse JSON-RPC request: %s", json_tokener_error_desc(jerr));

        BUFFER *error_payload = mcp_jsonrpc_build_error_payload(NULL, -32700, "Parse error", NULL, 0);
        mcp_websocket_send_payload(wsc, error_payload);
        buffer_free(error_payload);
        return;
    }

    BUFFER *response = NULL;
    if (json_object_is_type(request, json_type_array)) {
        // the responses are collected into one JSON array message
        BUFFER *batch = buffer_create(1024, NULL);
        response = mcp_jsonrpc_process_batch(mcpc, request, mcp_jsonrpc_batch_emit_array, batch);
        if (!response && buffer_strlen(batch)) {
            buffer_fast_strcat(batch, "]", 1);
            mcp_websocket_send_payload(wsc, batch);
        }
        buffer_free(batch);
    }
    else
        response = mcp_jsonrpc_process_single_request(mcpc, request, NULL);

    if (response) {
        mcp_websocket_send_payload(wsc, response);
        buffer_free(response);
    }

    json_object_put(request);
}

// WebSocket close handler for MCP
void mcp_websocket_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason) {
    if (!wsc) return;
    
    websocket_debug(wsc, "MCP client closing (code: %d, reason: %s)", code, reason ? reason : "none");
    
    mcp_websocket_release_context(wsc);
}

// WebSocket disconnect handler for MCP
void mcp_websocket_on_disconnect(struct websocket_server_client *wsc) {
    if (!wsc) return;
    
    websocket_debug(wsc, "MCP client disconnected");
    
    mcp_websocket_release_context(wsc);
}

// Register WebSocket callbacks for MCP
void mcp_websocket_adapter_initialize(void) {
    mcp_initialize_subsystem();
    netdata_log_info("MCP WebSocket adapter initialized");
}
