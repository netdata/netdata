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
    
    // Log the raw incoming message
    netdata_log_debug(D_MCP, "RCV: %s", message);
    
    // Only handle text messages
    if (opcode != WS_OPCODE_TEXT) {
        websocket_error(wsc, "Ignoring binary message - mcp supports only TEXT messages");
        return;
    }
    
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
    request = json_tokener_parse_verbose(message, &jerr);
    
    if (!request || jerr != json_tokener_success) {
        websocket_error(wsc, "Failed to parse JSON-RPC request: %s", json_tokener_error_desc(jerr));

        BUFFER *error_payload = mcp_jsonrpc_build_error_payload(NULL, -32700, "Parse error", NULL, 0);
        mcp_websocket_send_payload(wsc, error_payload);
        buffer_free(error_payload);
        return;
    }

    if (json_object_is_type(request, json_type_array)) {
        int len = (int)json_object_array_length(request);
        BUFFER **responses = NULL;
        size_t responses_used = 0;
        size_t responses_size = 0;

        for (int i = 0; i < len; i++) {
            struct json_object *req_item = json_object_array_get_idx(request, i);
            BUFFER *resp_item = mcp_jsonrpc_process_single_request(mcpc, req_item, NULL);
            if (resp_item) {
                if (responses_used == responses_size) {
                    size_t new_size = responses_size ? responses_size * 2 : 4;
                    BUFFER **tmp = reallocz(responses, new_size * sizeof(*tmp));
                    if (!tmp) {
                        buffer_free(resp_item);
                        continue;
                    }
                    responses = tmp;
                    responses_size = new_size;
                }
                responses[responses_used++] = resp_item;
            }
        }

        if (responses_used > 0) {
            size_t total_len = 2; // brackets
            for (size_t i = 0; i < responses_used; i++)
                total_len += buffer_strlen(responses[i]) + (i ? 1 : 0);

            BUFFER *batch = buffer_create(total_len + 32, NULL);
            buffer_fast_strcat(batch, "[", 1);
            for (size_t i = 0; i < responses_used; i++) {
                if (i)
                    buffer_fast_strcat(batch, ",", 1);
                const char *resp_text = buffer_tostring(responses[i]);
                size_t resp_len = buffer_strlen(responses[i]);
                buffer_fast_strcat(batch, resp_text, resp_len);
            }
            buffer_fast_strcat(batch, "]", 1);
            mcp_websocket_send_payload(wsc, batch);
            buffer_free(batch);
        }

        for (size_t i = 0; i < responses_used; i++)
            buffer_free(responses[i]);
        freez(responses);
    } else {
        BUFFER *response = mcp_jsonrpc_process_single_request(mcpc, request, NULL);
        if (response) {
            mcp_websocket_send_payload(wsc, response);
            buffer_free(response);
        }
    }

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
    mcp_initialize_subsystem();
    netdata_log_info("MCP WebSocket adapter initialized");
}
