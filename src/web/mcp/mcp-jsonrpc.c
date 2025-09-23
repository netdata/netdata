// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-jsonrpc.h"

#include <string.h>

static const size_t MCP_JSONRPC_RESPONSE_MAX_BYTES = 16 * 1024 * 1024;

static void buffer_append_json_id(BUFFER *out, struct json_object *id_obj) {
    if (!id_obj) {
        buffer_strcat(out, "null");
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

int mcp_jsonrpc_error_code(MCP_RETURN_CODE rc) {
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

BUFFER *mcp_jsonrpc_build_error_payload(struct json_object *id_obj, int code, const char *message,
                                        const struct mcp_response_chunk *chunks, size_t chunk_count) {
    BUFFER *out = buffer_create(512, NULL);
    buffer_strcat(out, "{\"jsonrpc\":\"2.0\",\"id\":");
    buffer_append_json_id(out, id_obj);
    buffer_strcat(out, ",\"error\":{\"code\":");
    buffer_sprintf(out, "%d", code);
    buffer_strcat(out, ",\"message\":");
    buffer_append_json_string_value(out, message ? message : "");

    if (chunk_count >= 1 && chunks && chunks[0].buffer && buffer_strlen(chunks[0].buffer)) {
        buffer_strcat(out, ",\"data\":");
        if (chunks[0].type == MCP_RESPONSE_CHUNK_JSON)
            buffer_fast_strcat(out, buffer_tostring(chunks[0].buffer), buffer_strlen(chunks[0].buffer));
        else
            buffer_append_json_string_value(out, buffer_tostring(chunks[0].buffer));
    }

    buffer_strcat(out, "}}");
    return out;
}

BUFFER *mcp_jsonrpc_build_success_payload(struct json_object *id_obj, const struct mcp_response_chunk *chunk) {
    const char *chunk_text = chunk && chunk->buffer ? buffer_tostring(chunk->buffer) : NULL;
    size_t chunk_len = chunk_text ? buffer_strlen(chunk->buffer) : 0;

    BUFFER *out = buffer_create(64 + chunk_len, NULL);
    buffer_strcat(out, "{\"jsonrpc\":\"2.0\",\"id\":");
    buffer_append_json_id(out, id_obj);
    buffer_strcat(out, ",\"result\":");
    if (chunk_text && chunk_len)
        buffer_fast_strcat(out, chunk_text, chunk_len);
    else
        buffer_strcat(out, "{}");
    buffer_strcat(out, "}");
    return out;
}

BUFFER *mcp_jsonrpc_process_single_request(MCP_CLIENT *mcpc, struct json_object *request, bool *had_error) {
    if (had_error)
        *had_error = false;

    if (!mcpc || !request)
        return NULL;

    struct json_object *id_obj = NULL;
    bool has_id = json_object_is_type(request, json_type_object) && json_object_object_get_ex(request, "id", &id_obj);

    if (!json_object_is_type(request, json_type_object))
        return mcp_jsonrpc_build_error_payload(has_id ? id_obj : NULL, -32600, "Invalid request", NULL, 0);

    struct json_object *jsonrpc_obj = NULL;
    if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc_obj) ||
        !json_object_is_type(jsonrpc_obj, json_type_string) ||
        strcmp(json_object_get_string(jsonrpc_obj), "2.0") != 0) {
        return mcp_jsonrpc_build_error_payload(has_id ? id_obj : NULL, -32600, "Invalid or missing jsonrpc version", NULL, 0);
    }

    struct json_object *method_obj = NULL;
    if (!json_object_object_get_ex(request, "method", &method_obj) ||
        !json_object_is_type(method_obj, json_type_string)) {
        return mcp_jsonrpc_build_error_payload(has_id ? id_obj : NULL, -32600, "Missing or invalid method", NULL, 0);
    }
    const char *method = json_object_get_string(method_obj);

    struct json_object *params_obj = NULL;
    bool params_created = false;
    if (json_object_object_get_ex(request, "params", &params_obj)) {
        if (!json_object_is_type(params_obj, json_type_object)) {
            return mcp_jsonrpc_build_error_payload(has_id ? id_obj : NULL, -32602, "Params must be an object", NULL, 0);
        }
    } else {
        params_obj = json_object_new_object();
        params_created = true;
    }

    MCP_RETURN_CODE rc = mcp_dispatch_method(mcpc, method, params_obj, has_id ? 1 : 0);

    if (params_created)
        json_object_put(params_obj);

    size_t total_bytes = mcp_client_response_size(mcpc);
    if (total_bytes > MCP_JSONRPC_RESPONSE_MAX_BYTES) {
        BUFFER *payload = mcp_jsonrpc_build_error_payload(has_id ? id_obj : NULL,
                                                           -32001,
                                                           "Response too large for transport",
                                                           NULL, 0);
        mcp_client_release_response(mcpc);
        mcp_client_clear_error(mcpc);
        if (had_error)
            *had_error = true;
        return payload;
    }

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
            payload = mcp_jsonrpc_build_error_payload(id_obj, -32603, "Empty response", NULL, 0);
            if (had_error)
                *had_error = true;
        }
        else if (chunk_count > 1 || chunks[0].type != MCP_RESPONSE_CHUNK_JSON) {
            payload = mcp_jsonrpc_build_error_payload(id_obj, -32002, "Streaming responses not supported on this transport", NULL, 0);
            if (had_error)
                *had_error = true;
        }
        else {
            payload = mcp_jsonrpc_build_success_payload(id_obj, &chunks[0]);
        }
    } else {
        const char *message = mcp_client_error_message(mcpc);
        if (!message)
            message = MCP_RETURN_CODE_2str(rc);
        payload = mcp_jsonrpc_build_error_payload(id_obj, mcp_jsonrpc_error_code(rc), message, chunks, chunk_count);
        if (had_error)
            *had_error = true;
    }

    mcp_client_release_response(mcpc);
    mcp_client_clear_error(mcpc);
    return payload;
}

BUFFER *mcp_jsonrpc_build_batch_response(BUFFER **responses, size_t count) {
    if (!responses || count == 0)
        return NULL;

    size_t total_len = 2; // []
    for (size_t i = 0; i < count; i++) {
        if (!responses[i])
            continue;
        total_len += buffer_strlen(responses[i]);
        if (i)
            total_len += 1;
    }

    BUFFER *batch = buffer_create(total_len + 32, NULL);
    buffer_strcat(batch, "[");
    bool first = true;
    for (size_t i = 0; i < count; i++) {
        if (!responses[i])
            continue;
        if (!first)
            buffer_strcat(batch, ",");
        first = false;
        const char *resp_text = buffer_tostring(responses[i]);
        size_t resp_len = buffer_strlen(responses[i]);
        buffer_fast_strcat(batch, resp_text, resp_len);
    }
    buffer_strcat(batch, "]");
    return batch;
}
