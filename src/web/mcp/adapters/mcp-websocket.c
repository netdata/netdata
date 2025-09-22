// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-websocket.h"
#include "web/websocket/websocket-internal.h"

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

static int mcp_jsonrpc_error_code(MCP_RETURN_CODE rc) {
    switch (rc) {
        case MCP_RC_INVALID_PARAMS:
            return -32602;
        case MCP_RC_NOT_FOUND:
        case MCP_RC_NOT_IMPLEMENTED:
            return -32601;
        case MCP_RC_BAD_REQUEST:
            return -32600;
        case MCP_RC_INTERNAL_ERROR:
            return -32603;
        case MCP_RC_OK:
            return 0;
        case MCP_RC_ERROR:
        default:
            return -32000;
    }
}

static const size_t MCP_WEBSOCKET_RESPONSE_MAX_BYTES = 16 * 1024 * 1024;

static void buffer_append_json_id(BUFFER *out, struct json_object *id_obj) {
    if (!id_obj) {
        buffer_fast_strcat(out, "null", 4);
        return;
    }

    const char *id_text = json_object_to_json_string_ext(id_obj, JSON_C_TO_STRING_PLAIN);
    if (!id_text)
        id_text = "null";
    buffer_fast_strcat(out, id_text, strlen(id_text));
}

static void buffer_append_json_string_value(BUFFER *out, const char *text) {
    struct json_object *tmp = json_object_new_string(text ? text : "");
    const char *payload = json_object_to_json_string_ext(tmp, JSON_C_TO_STRING_PLAIN);
    if (payload)
        buffer_fast_strcat(out, payload, strlen(payload));
    json_object_put(tmp);
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

static BUFFER *mcp_websocket_build_error_payload(struct json_object *id_obj, int code, const char *message, const struct mcp_response_chunk *chunks, size_t chunk_count) {
    BUFFER *out = buffer_create(512, NULL);
    buffer_fast_strcat(out, "{\"jsonrpc\":\"2.0\",\"id\":", 25);
    buffer_append_json_id(out, id_obj);
    buffer_fast_strcat(out, ",\"error\":{\"code\":", 19);
    buffer_sprintf(out, "%d", code);
    buffer_fast_strcat(out, ",\"message\":", 13);
    buffer_append_json_string_value(out, message ? message : "");

    if (chunk_count >= 1 && chunks && chunks[0].buffer && buffer_strlen(chunks[0].buffer)) {
        buffer_fast_strcat(out, ",\"data\":", 9);
        if (chunks[0].type == MCP_RESPONSE_CHUNK_JSON)
            buffer_fast_strcat(out, buffer_tostring(chunks[0].buffer), buffer_strlen(chunks[0].buffer));
        else
            buffer_append_json_string_value(out, buffer_tostring(chunks[0].buffer));
    }

    buffer_fast_strcat(out, "}}", 2);
    return out;
}

static BUFFER *mcp_websocket_build_success_payload(struct json_object *id_obj, const struct mcp_response_chunk *chunk) {
    const char *chunk_text = chunk && chunk->buffer ? buffer_tostring(chunk->buffer) : NULL;
    size_t chunk_len = chunk_text ? buffer_strlen(chunk->buffer) : 0;

    BUFFER *out = buffer_create(64 + chunk_len, NULL);
    buffer_fast_strcat(out, "{\"jsonrpc\":\"2.0\",\"id\":", 25);
    buffer_append_json_id(out, id_obj);
    buffer_fast_strcat(out, ",\"result\":", 11);
    if (chunk_text && chunk_len)
        buffer_fast_strcat(out, chunk_text, chunk_len);
    else
        buffer_fast_strcat(out, "{}", 2);
    buffer_fast_strcat(out, "}", 1);
    return out;
}

static BUFFER *mcp_websocket_process_single_request(MCP_CLIENT *mcpc, struct json_object *request, bool *had_error) {
    if (had_error)
        *had_error = false;

    if (!mcpc || !request)
        return NULL;

    struct json_object *id_obj = NULL;
    bool has_id = json_object_is_type(request, json_type_object) && json_object_object_get_ex(request, "id", &id_obj);

    if (!json_object_is_type(request, json_type_object))
        return mcp_websocket_build_error_payload(has_id ? id_obj : NULL, -32600, "Invalid request", NULL, 0);

    struct json_object *jsonrpc_obj = NULL;
    if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc_obj) ||
        !json_object_is_type(jsonrpc_obj, json_type_string) ||
        strcmp(json_object_get_string(jsonrpc_obj), "2.0") != 0) {
        return mcp_websocket_build_error_payload(has_id ? id_obj : NULL, -32600, "Invalid or missing jsonrpc version", NULL, 0);
    }

    struct json_object *method_obj = NULL;
    if (!json_object_object_get_ex(request, "method", &method_obj) ||
        !json_object_is_type(method_obj, json_type_string)) {
        return mcp_websocket_build_error_payload(has_id ? id_obj : NULL, -32600, "Missing or invalid method", NULL, 0);
    }
    const char *method = json_object_get_string(method_obj);

    struct json_object *params_obj = NULL;
    bool params_created = false;
    if (json_object_object_get_ex(request, "params", &params_obj)) {
        if (!json_object_is_type(params_obj, json_type_object)) {
            return mcp_websocket_build_error_payload(has_id ? id_obj : NULL, -32602, "Params must be an object", NULL, 0);
        }
    } else {
        params_obj = json_object_new_object();
        params_created = true;
    }

    MCP_RETURN_CODE rc = mcp_dispatch_method(mcpc, method, params_obj, has_id ? 1 : 0);

    if (params_created)
        json_object_put(params_obj);

    size_t total_bytes = mcp_client_response_size(mcpc);
    if (total_bytes > MCP_WEBSOCKET_RESPONSE_MAX_BYTES) {
        BUFFER *payload = mcp_websocket_build_error_payload(has_id ? id_obj : NULL,
                                                           -32001,
                                                           "Response too large for WebSocket transport",
                                                           NULL, 0);
        mcp_client_release_response(mcpc);
        mcp_client_clear_error(mcpc);
        if (had_error)
            *had_error = true;
        return payload;
    }

    // Notifications do not require a response
    if (!has_id) {
        mcp_client_release_response(mcpc);
        mcp_client_clear_error(mcpc);
        return NULL;
    }

    const struct mcp_response_chunk *chunks = mcp_client_response_chunks(mcpc);
    size_t chunk_count = mcp_client_response_chunk_count(mcpc);

    BUFFER *payload = NULL;

    if (rc == MCP_RC_OK && !mcpc->last_response_error) {
        if (!chunks || chunk_count == 0) {
            payload = mcp_websocket_build_error_payload(id_obj, -32603, "Empty response", NULL, 0);
            if (had_error)
                *had_error = true;
        }
        else if (chunk_count > 1 || chunks[0].type != MCP_RESPONSE_CHUNK_JSON) {
            payload = mcp_websocket_build_error_payload(id_obj, -32002, "Streaming responses not supported on WebSocket", NULL, 0);
            if (had_error)
                *had_error = true;
        }
        else {
            payload = mcp_websocket_build_success_payload(id_obj, &chunks[0]);
        }
    } else {
        const char *message = mcp_client_error_message(mcpc);
        if (!message)
            message = MCP_RETURN_CODE_2str(rc);
        payload = mcp_websocket_build_error_payload(id_obj, mcp_jsonrpc_error_code(rc), message, chunks, chunk_count);
        if (had_error)
            *had_error = true;
    }

    mcp_client_release_response(mcpc);
    mcp_client_clear_error(mcpc);
    return payload;
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

        BUFFER *error_payload = mcp_websocket_build_error_payload(NULL, -32700, "Parse error", NULL, 0);
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
            BUFFER *resp_item = mcp_websocket_process_single_request(mcpc, req_item, NULL);
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
        BUFFER *response = mcp_websocket_process_single_request(mcpc, request, NULL);
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
