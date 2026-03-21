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

// ---------------------------------------------------------------------------
// MCP HTTP session management (Streamable HTTP transport, MCP 2025-03-26)
// ---------------------------------------------------------------------------

#define MCP_HTTP_SESSION_TTL_SECONDS 3600  // 1-hour inactivity expiry

typedef struct {
    MCP_PROTOCOL_VERSION protocol_version;
    bool ready;
    STRING *client_name;
    STRING *client_version;
    MCP_LOGGING_LEVEL logging_level;
    time_t last_accessed;
} MCP_HTTP_SESSION;

static DICTIONARY *mcp_http_sessions = NULL;
static SPINLOCK mcp_http_sessions_lock = SPINLOCK_INITIALIZER;

static void mcp_http_session_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value,
                                       void *data __maybe_unused) {
    MCP_HTTP_SESSION *s = (MCP_HTTP_SESSION *)value;
    string_freez(s->client_name);
    string_freez(s->client_version);
    s->client_name = NULL;
    s->client_version = NULL;
}

// Must be called with mcp_http_sessions_lock held.
static void mcp_http_sessions_init_nolock(void) {
    if (mcp_http_sessions)
        return;

    mcp_http_sessions = dictionary_create_advanced(
        DICT_OPTION_FIXED_SIZE | DICT_OPTION_SINGLE_THREADED,
        NULL,
        sizeof(MCP_HTTP_SESSION));

    dictionary_register_delete_callback(mcp_http_sessions, mcp_http_session_delete_cb, NULL);
}

// Must be called with mcp_http_sessions_lock held.
static void mcp_http_sessions_cleanup_expired_nolock(void) {
    if (!mcp_http_sessions)
        return;

    time_t now = now_realtime_sec();
    MCP_HTTP_SESSION *s;
    dfe_start_write(mcp_http_sessions, s) {
        if (now - s->last_accessed > MCP_HTTP_SESSION_TTL_SECONDS)
            dictionary_del(mcp_http_sessions, s_dfe.name);
    }
    dfe_done(s);

    dictionary_garbage_collect(mcp_http_sessions);
}

// Generate a new session ID and persist the MCP_CLIENT state.
// session_id_out must be at least UUID_STR_LEN bytes.
static void mcp_http_session_create(MCP_CLIENT *mcpc, char *session_id_out) {
    nd_uuid_t uuid;
    os_uuid_generate_random(&uuid);
    nd_uuid_unparse_lower(uuid, session_id_out);

    MCP_HTTP_SESSION s = {
        .protocol_version = mcpc->protocol_version,
        .ready            = mcpc->ready,
        .client_name      = string_dup(mcpc->client_name),
        .client_version   = string_dup(mcpc->client_version),
        .logging_level    = mcpc->logging_level,
        .last_accessed    = now_realtime_sec(),
    };

    spinlock_lock(&mcp_http_sessions_lock);
    mcp_http_sessions_init_nolock();

    // Extremely unlikely UUID collision: clean up old STRING pointers before overwriting.
    MCP_HTTP_SESSION *existing = (MCP_HTTP_SESSION *)dictionary_get(mcp_http_sessions, session_id_out);
    if (unlikely(existing)) {
        string_freez(existing->client_name);
        string_freez(existing->client_version);
        existing->client_name = NULL;
        existing->client_version = NULL;
    }

    dictionary_set(mcp_http_sessions, session_id_out, &s, sizeof(s));
    mcp_http_sessions_cleanup_expired_nolock();

    spinlock_unlock(&mcp_http_sessions_lock);
}

// Restore session state into mcpc.  Returns true on success, false if not found.
static bool mcp_http_session_restore(const char *session_id, MCP_CLIENT *mcpc) {
    if (!session_id || !*session_id || !mcpc)
        return false;

    spinlock_lock(&mcp_http_sessions_lock);
    mcp_http_sessions_init_nolock();

    MCP_HTTP_SESSION *s = (MCP_HTTP_SESSION *)dictionary_get(mcp_http_sessions, session_id);
    if (!s) {
        spinlock_unlock(&mcp_http_sessions_lock);
        return false;
    }

    mcpc->protocol_version = s->protocol_version;
    mcpc->ready            = s->ready;
    mcpc->logging_level    = s->logging_level;

    string_freez(mcpc->client_name);
    mcpc->client_name = string_dup(s->client_name);

    string_freez(mcpc->client_version);
    mcpc->client_version = string_dup(s->client_version);

    s->last_accessed = now_realtime_sec();

    spinlock_unlock(&mcp_http_sessions_lock);
    return true;
}

// Persist updated MCP_CLIENT state back into an existing session.
static void mcp_http_session_update(const char *session_id, MCP_CLIENT *mcpc) {
    if (!session_id || !*session_id || !mcpc)
        return;

    spinlock_lock(&mcp_http_sessions_lock);
    if (!mcp_http_sessions) {
        spinlock_unlock(&mcp_http_sessions_lock);
        return;
    }

    MCP_HTTP_SESSION *s = (MCP_HTTP_SESSION *)dictionary_get(mcp_http_sessions, session_id);
    if (s) {
        s->protocol_version = mcpc->protocol_version;
        s->ready            = mcpc->ready;
        s->logging_level    = mcpc->logging_level;
        s->last_accessed    = now_realtime_sec();

        if (string_strcmp(s->client_name, string2str(mcpc->client_name)) != 0) {
            string_freez(s->client_name);
            s->client_name = string_dup(mcpc->client_name);
        }
        if (string_strcmp(s->client_version, string2str(mcpc->client_version)) != 0) {
            string_freez(s->client_version);
            s->client_version = string_dup(mcpc->client_version);
        }
    }

    spinlock_unlock(&mcp_http_sessions_lock);
}

// Check whether a session ID exists (call with lock NOT held).
static bool mcp_http_session_exists(const char *session_id) {
    if (!session_id || !*session_id)
        return false;

    spinlock_lock(&mcp_http_sessions_lock);
    mcp_http_sessions_init_nolock();
    bool found = dictionary_get(mcp_http_sessions, session_id) != NULL;
    spinlock_unlock(&mcp_http_sessions_lock);
    return found;
}

// ---------------------------------------------------------------------------

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
        mcp_api_key_verify(api_key_buffer, false)) {  // silent=false for MCP requests
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

    // ------------------------------------------------------------------
    // Session lookup: if the client sent Mcp-Session-Id we must honour it
    // ------------------------------------------------------------------
    const char *incoming_session_id = w->mcp_session_id;  // parsed from Mcp-Session-Id header
    bool has_incoming_session = incoming_session_id && *incoming_session_id;

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

    // Detect the top-level method so we can apply session logic correctly.
    // For batch requests we read the method from the first item in the array;
    // individual session validation is per-item but batch sessions are rare.
    const char *method = NULL;
    struct json_object *method_obj = NULL;
    if (json_object_is_type(root, json_type_object)) {
        if (json_object_object_get_ex(root, "method", &method_obj))
            method = json_object_get_string(method_obj);
    }

    // If the client provided a session ID for a non-initialize request, validate it.
    // Unknown session IDs are rejected with HTTP 404 per the MCP Streamable HTTP spec.
    if (has_incoming_session && method && strcmp(method, "initialize") != 0) {
        if (!mcp_http_session_exists(incoming_session_id)) {
            json_object_put(root);
            BUFFER *payload = mcp_jsonrpc_build_error_payload(
                NULL, -32001, "Session not found or expired", NULL, 0);
            return mcp_http_prepare_error_response(w, payload, HTTP_RESP_NOT_FOUND);
        }
    }

    MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, w);
    if (!mcpc) {
        json_object_put(root);
        BUFFER *payload = mcp_jsonrpc_build_error_payload(NULL, -32603, "Failed to allocate MCP client", NULL, 0);
        return mcp_http_prepare_error_response(w, payload, HTTP_RESP_INTERNAL_SERVER_ERROR);
    }
    mcpc->user_auth = &w->user_auth;

    // Restore session state for non-initialize requests that carry a session ID.
    if (has_incoming_session && method && strcmp(method, "initialize") != 0)
        mcp_http_session_restore(incoming_session_id, mcpc);

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

    // ------------------------------------------------------------------
    // Session management post-processing
    // ------------------------------------------------------------------
    if (method && strcmp(method, "initialize") == 0 && result_code == HTTP_RESP_OK) {
        // Create a new session and advertise the session ID to the client.
        char new_session_id[UUID_STR_LEN];
        mcp_http_session_create(mcpc, new_session_id);
        buffer_sprintf(w->response.header, "Mcp-Session-Id: %s\r\n", new_session_id);
    } else if (has_incoming_session &&
               (result_code == HTTP_RESP_OK || result_code == HTTP_RESP_ACCEPTED)) {
        // Persist any state changes (e.g. ready flag set by notifications/initialized).
        mcp_http_session_update(incoming_session_id, mcpc);
    }

    json_object_put(root);
    mcp_free_client(mcpc);
    return result_code;
}
