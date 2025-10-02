// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-http.h"

#include "web/server/web_client.h"
#include "web/mcp/mcp-jsonrpc.h"
#include "web/mcp/mcp.h"
#include "web/mcp/adapters/mcp-sse.h"
#include "mcp-http-common.h"

#include "web/api/mcp_auth.h"

#include "libnetdata/libnetdata.h"
#include "libnetdata/http/http_defs.h"
#include "libnetdata/http/content_type.h"

#include <stdbool.h>
#include <json-c/json.h>
#include <string.h>
#include <strings.h>

#define IS_PARAM_SEPARATOR(c) ((c) == '&' || (c) == '\0')

static const char *mcp_http_body(struct web_client *w, size_t *len) {
    if (!w || !w->payload)
        return NULL;

    const char *body = buffer_tostring(w->payload);
    if (!body)
        return NULL;

    if (len)
        *len = buffer_strlen(w->payload);
    return body;
}

static bool mcp_http_accepts_sse(struct web_client *w) {
    if (!w)
        return false;

    if (web_client_flag_check(w, WEB_CLIENT_FLAG_ACCEPT_SSE))
        return true;

    if (!w->url_query_string_decoded)
        return false;

    const char *qs = buffer_tostring(w->url_query_string_decoded);
    if (!qs || !*qs)
        return false;

    if (*qs == '?')
        qs++;

    if (!*qs)
        return false;

    const char *param = strstr(qs, "transport=");
    if (!param)
        return false;

    param += strlen("transport=");
    if (strncasecmp(param, "sse", 3) == 0 && IS_PARAM_SEPARATOR(param[3]))
        return true;

    return false;
}

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
static void mcp_http_apply_api_key(struct web_client *w) {
    if (web_client_has_mcp_preview_key(w)) {
        web_client_set_permissions(w, HTTP_ACCESS_ALL, HTTP_USER_ROLE_ADMIN, USER_AUTH_METHOD_GOD);
        return;
    }

    char api_key_buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH + 1];
    if (mcp_http_extract_api_key(w, api_key_buffer, sizeof(api_key_buffer)) &&
        mcp_api_key_verify(api_key_buffer)) {
        web_client_set_permissions(w, HTTP_ACCESS_ALL, HTTP_USER_ROLE_ADMIN, USER_AUTH_METHOD_GOD);
    }
}
#endif

static void mcp_http_write_json_payload(struct web_client *w, BUFFER *payload) {
    if (!w)
        return;

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;

    if (payload && buffer_strlen(payload))
        buffer_fast_strcat(w->response.data, buffer_tostring(payload), buffer_strlen(payload));
}

static int mcp_http_prepare_error_response(struct web_client *w, BUFFER *payload, int http_code) {
    w->response.code = http_code;
    mcp_http_write_json_payload(w, payload);
    if (payload)
        buffer_free(payload);
    return http_code;
}

int mcp_http_handle_request(struct rrdhost *host __maybe_unused, struct web_client *w) {
    if (!w)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;

    if (w->mode != HTTP_REQUEST_MODE_POST && w->mode != HTTP_REQUEST_MODE_GET) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Unsupported HTTP method for /mcp\n");
        w->response.data->content_type = CT_TEXT_PLAIN;
        w->response.code = HTTP_RESP_METHOD_NOT_ALLOWED;
        return w->response.code;
    }

#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
    mcp_http_apply_api_key(w);
#endif

    size_t body_len = 0;
    const char *body = mcp_http_body(w, &body_len);
    if (!body || !body_len) {
        BUFFER *payload = mcp_jsonrpc_build_error_payload(NULL, -32600, "Empty request body", NULL, 0);
        return mcp_http_prepare_error_response(w, payload, HTTP_RESP_BAD_REQUEST);
    }

    enum json_tokener_error jerr = json_tokener_success;
    struct json_object *root = json_tokener_parse_verbose(body, &jerr);
    if (!root || jerr != json_tokener_success) {
        BUFFER *payload = mcp_jsonrpc_build_error_payload(NULL, -32700, json_tokener_error_desc(jerr), NULL, 0);
        if (root)
            json_object_put(root);
        return mcp_http_prepare_error_response(w, payload, HTTP_RESP_BAD_REQUEST);
    }

    MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, w);
    if (!mcpc) {
        json_object_put(root);
        BUFFER *payload = mcp_jsonrpc_build_error_payload(NULL, -32603, "Failed to allocate MCP client", NULL, 0);
        return mcp_http_prepare_error_response(w, payload, HTTP_RESP_INTERNAL_SERVER_ERROR);
    }
    mcpc->user_auth = &w->user_auth;

    bool wants_sse = mcp_http_accepts_sse(w);

    int result_code = HTTP_RESP_INTERNAL_SERVER_ERROR;

    if (wants_sse) {
        mcpc->transport = MCP_TRANSPORT_SSE;
        mcpc->capabilities = MCP_CAPABILITY_ASYNC_COMMUNICATION |
                             MCP_CAPABILITY_SUBSCRIPTIONS |
                             MCP_CAPABILITY_NOTIFICATIONS;
        result_code = mcp_sse_serialize_response(w, mcpc, root);
    } else {
        BUFFER *response_payload = NULL;
        bool has_response = false;

        if (json_object_is_type(root, json_type_array)) {
            size_t len = json_object_array_length(root);
            BUFFER **responses = NULL;
            size_t responses_used = 0;
            size_t responses_size = 0;

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

            if (responses_used) {
                response_payload = mcp_jsonrpc_build_batch_response(responses, responses_used);
                has_response = response_payload && buffer_strlen(response_payload);
            }

            for (size_t i = 0; i < responses_used; i++)
                buffer_free(responses[i]);
            freez(responses);
        } else {
            response_payload = mcp_jsonrpc_process_single_request(mcpc, root, NULL);
            has_response = response_payload && buffer_strlen(response_payload);
        }

        if (response_payload) {
            mcp_http_write_json_payload(w, response_payload);
        } else {
            buffer_flush(w->response.data);
            mcp_http_disable_compression(w);
            w->response.data->content_type = CT_APPLICATION_JSON;
            buffer_flush(w->response.header);
        }

        w->response.code = has_response ? HTTP_RESP_OK : HTTP_RESP_ACCEPTED;

        if (response_payload)
            buffer_free(response_payload);

        result_code = w->response.code;
    }

    json_object_put(root);
    mcp_free_client(mcpc);
    return result_code;
}
