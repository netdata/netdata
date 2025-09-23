// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-sse.h"

#include "web/server/web_client.h"
#include "web/mcp/mcp-jsonrpc.h"
#include "web/mcp/mcp.h"
#include "mcp-http-common.h"

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
#include "web/mcp/mcp-api-key.h"
#endif

#include "libnetdata/libnetdata.h"
#include "libnetdata/http/http_defs.h"
#include "libnetdata/http/content_type.h"

#include <json-c/json.h>

static void mcp_sse_disable_compression(struct web_client *w) {
    if (!w)
        return;

    web_client_flag_clear(w, WEB_CLIENT_ENCODING_GZIP);
    web_client_flag_clear(w, WEB_CLIENT_ENCODING_DEFLATE);
    web_client_flag_clear(w, WEB_CLIENT_CHUNKED_TRANSFER);
    w->response.zoutput = false;
    w->response.zinitialized = false;
}

static void mcp_sse_add_common_headers(struct web_client *w) {
    if (!w)
        return;

    buffer_flush(w->response.header);
    buffer_strcat(w->response.header, "Cache-Control: no-cache\r\n");
    buffer_strcat(w->response.header, "Connection: keep-alive\r\n");
}

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
static void mcp_sse_apply_api_key(struct web_client *w) {
    char api_key_buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH + 1];
    if (mcp_http_extract_api_key(w, api_key_buffer, sizeof(api_key_buffer)) &&
        mcp_api_key_verify(api_key_buffer)) {
        web_client_set_permissions(w, HTTP_ACCESS_ALL, HTTP_USER_ROLE_ADMIN, USER_AUTH_METHOD_GOD);
    }
}
#endif

static void mcp_sse_append_event(BUFFER *out, const char *event, const char *data) {
    if (!out || !event)
        return;

    buffer_strcat(out, "event: ");
    buffer_strcat(out, event);
    buffer_strcat(out, "\n");

    if (data && *data) {
        buffer_strcat(out, "data: ");
        buffer_strcat(out, data);
        buffer_strcat(out, "\n");
    }

    buffer_strcat(out, "\n");
}

static void mcp_sse_append_buffer_event(BUFFER *out, const char *event, BUFFER *payload) {
    if (!out || !event || !payload)
        return;

    buffer_strcat(out, "event: ");
    buffer_strcat(out, event);
    buffer_strcat(out, "\n");

    buffer_strcat(out, "data: ");
    buffer_fast_strcat(out, buffer_tostring(payload), buffer_strlen(payload));
    buffer_strcat(out, "\n\n");
}

int mcp_sse_serialize_response(struct web_client *w, MCP_CLIENT *mcpc, struct json_object *root) {
    if (!w || !mcpc || !root)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;

    BUFFER **responses = NULL;
    size_t responses_used = 0;
    size_t responses_size = 0;

    if (json_object_is_type(root, json_type_array)) {
        size_t len = json_object_array_length(root);
        for (size_t i = 0; i < len; i++) {
            struct json_object *req_item = json_object_array_get_idx(root, i);
            BUFFER *resp_item = mcp_jsonrpc_process_single_request(mcpc, req_item, NULL);
            if (!resp_item)
                continue;

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
    } else {
        BUFFER *resp = mcp_jsonrpc_process_single_request(mcpc, root, NULL);
        if (resp) {
            responses = reallocz(responses, sizeof(*responses));
            if (responses)
                responses[responses_used++] = resp;
            else
                buffer_free(resp);
        }
    }

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_TEXT_EVENT_STREAM;
    mcp_sse_disable_compression(w);
    mcp_sse_add_common_headers(w);

    for (size_t i = 0; i < responses_used; i++) {
        if (!responses[i])
            continue;
        mcp_sse_append_buffer_event(w->response.data, "message", responses[i]);
        buffer_free(responses[i]);
    }
    freez(responses);

    mcp_sse_append_event(w->response.data, "complete", "{}");

    w->response.code = HTTP_RESP_OK;
    return w->response.code;
}

int mcp_sse_handle_request(struct rrdhost *host __maybe_unused, struct web_client *w) {
    if (!w)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;

    if (w->mode != HTTP_REQUEST_MODE_GET && w->mode != HTTP_REQUEST_MODE_POST) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Unsupported HTTP method for /sse\n");
        w->response.data->content_type = CT_TEXT_PLAIN;
        w->response.code = HTTP_RESP_METHOD_NOT_ALLOWED;
        return w->response.code;
    }

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
    mcp_sse_apply_api_key(w);
#endif

    size_t body_len = 0;
    const char *body = NULL;
    if (w->payload)
        body = buffer_tostring(w->payload);
    if (body)
        body_len = buffer_strlen(w->payload);

    if (!body || !body_len) {
        buffer_flush(w->response.data);
        w->response.data->content_type = CT_TEXT_EVENT_STREAM;
        mcp_sse_disable_compression(w);
        mcp_sse_add_common_headers(w);
        mcp_sse_append_event(w->response.data, "error", "Empty request body");
        w->response.code = HTTP_RESP_BAD_REQUEST;
        return w->response.code;
    }

    enum json_tokener_error jerr = json_tokener_success;
    struct json_object *root = json_tokener_parse_verbose(body, &jerr);
    if (!root || jerr != json_tokener_success) {
        BUFFER *payload = mcp_jsonrpc_build_error_payload(NULL, -32700, json_tokener_error_desc(jerr), NULL, 0);
        buffer_flush(w->response.data);
        w->response.data->content_type = CT_TEXT_EVENT_STREAM;
        mcp_sse_disable_compression(w);
        mcp_sse_add_common_headers(w);
        if (payload) {
            mcp_sse_append_buffer_event(w->response.data, "error", payload);
            buffer_free(payload);
        } else {
            mcp_sse_append_event(w->response.data, "error", json_tokener_error_desc(jerr));
        }
        w->response.code = HTTP_RESP_BAD_REQUEST;
        if (root)
            json_object_put(root);
        return w->response.code;
    }

    MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_SSE, w);
    if (!mcpc) {
        json_object_put(root);
        buffer_flush(w->response.data);
        w->response.data->content_type = CT_TEXT_EVENT_STREAM;
        mcp_sse_disable_compression(w);
        mcp_sse_add_common_headers(w);
        mcp_sse_append_event(w->response.data, "error", "Failed to allocate MCP client");
        w->response.code = HTTP_RESP_INTERNAL_SERVER_ERROR;
        return w->response.code;
    }
    mcpc->user_auth = &w->user_auth;

    int rc = mcp_sse_serialize_response(w, mcpc, root);

    json_object_put(root);
    mcp_free_client(mcpc);
    return rc;
}
