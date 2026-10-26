// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-jsonrpc.h"

#include <string.h>

static const size_t MCP_JSONRPC_RESPONSE_MAX_BYTES = 16 * 1024 * 1024;
static const size_t MCP_JSONRPC_BATCH_MAX_ENTRIES = 64;

struct json_object *mcp_jsonrpc_parse_request(const char *request, size_t length, enum json_tokener_error *error)
{
    struct json_tokener *tokener = json_tokener_new();
    if (!tokener) {
        *error = json_tokener_error_parse_unexpected;
        return NULL;
    }

    struct json_object *root = json_tokener_parse_ex(tokener, request, -1);
    *error = json_tokener_get_error(tokener);

    if (!root && *error == json_tokener_success) {
        json_tokener_reset(tokener);
        json_tokener_set_flags(tokener, JSON_TOKENER_STRICT);
        root = json_tokener_parse_ex(tokener, request, -1);
        *error = json_tokener_get_error(tokener);

        if (*error == json_tokener_success && json_tokener_get_parse_end(tokener) != length)
            *error = json_tokener_error_parse_unexpected;
    }

    json_tokener_free(tokener);
    return root;
}

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

// Check the shape of a request without running it. Returns the error payload for a malformed
// request, or NULL when it is well-formed; *id_obj is set when the request carries an id.
static BUFFER *mcp_jsonrpc_validate_request(struct json_object *request, struct json_object **id_obj) {
    *id_obj = NULL;

    if (!json_object_is_type(request, json_type_object))
        return mcp_jsonrpc_build_error_payload(NULL, -32600, "Invalid request", NULL, 0);

    // an id is a string, a number or null
    if (json_object_object_get_ex(request, "id", id_obj) && *id_obj &&
        !json_object_is_type(*id_obj, json_type_string) &&
        !json_object_is_type(*id_obj, json_type_int) &&
        !json_object_is_type(*id_obj, json_type_double)) {
        *id_obj = NULL;
        return mcp_jsonrpc_build_error_payload(NULL, -32600, "Invalid id", NULL, 0);
    }

    struct json_object *jsonrpc_obj = NULL;
    if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc_obj) ||
        !json_object_is_type(jsonrpc_obj, json_type_string) ||
        strcmp(json_object_get_string(jsonrpc_obj), "2.0") != 0) {
        return mcp_jsonrpc_build_error_payload(*id_obj, -32600, "Invalid or missing jsonrpc version", NULL, 0);
    }

    struct json_object *method_obj = NULL;
    if (!json_object_object_get_ex(request, "method", &method_obj) ||
        !json_object_is_type(method_obj, json_type_string)) {
        return mcp_jsonrpc_build_error_payload(*id_obj, -32600, "Missing or invalid method", NULL, 0);
    }

    struct json_object *params_obj = NULL;
    if (json_object_object_get_ex(request, "params", &params_obj) &&
        !json_object_is_type(params_obj, json_type_object)) {
        return mcp_jsonrpc_build_error_payload(*id_obj, -32602, "Params must be an object", NULL, 0);
    }

    return NULL;
}

BUFFER *mcp_jsonrpc_process_single_request(MCP_CLIENT *mcpc, struct json_object *request, bool *had_error) {
    if (had_error)
        *had_error = false;

    if (!mcpc)
        return NULL;

    struct json_object *id_obj = NULL;
    BUFFER *invalid = mcp_jsonrpc_validate_request(request, &id_obj);
    if (invalid)
        return invalid;

    bool has_id = json_object_object_get_ex(request, "id", NULL);

    struct json_object *method_obj = NULL;
    json_object_object_get_ex(request, "method", &method_obj);
    const char *method = json_object_get_string(method_obj);

    struct json_object *params_obj = NULL;
    bool params_created = false;
    if (!json_object_object_get_ex(request, "params", &params_obj)) {
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

void mcp_jsonrpc_batch_emit_array(BUFFER *response, void *data) {
    BUFFER *wb = data;
    buffer_fast_strcat(wb, buffer_strlen(wb) ? "," : "[", 1);
    buffer_fast_strcat(wb, buffer_tostring(response), buffer_strlen(response));
}

static BUFFER *mcp_jsonrpc_process_batch_limited(MCP_CLIENT *mcpc, struct json_object *batch,
                                                 mcp_jsonrpc_batch_emit_t emit, void *data,
                                                 size_t max_entries, size_t max_bytes) {
    size_t len = json_object_array_length(batch);
    if (!len)
        return mcp_jsonrpc_build_error_payload(NULL, -32600, "Invalid request", NULL, 0);

    if (len > max_entries) {
        char message[100];
        snprintfz(message, sizeof(message), "Batch has %zu requests, the limit is %zu", len, max_entries);
        return mcp_jsonrpc_build_error_payload(NULL, -32600, message, NULL, 0);
    }

    // Once the responses reach max_bytes, the remaining requests are not run: each one with an id
    // gets a small error instead. Notifications get nothing, malformed requests still get their error.
    size_t used = 0;
    bool exhausted = false;

    for (size_t i = 0; i < len; i++) {
        struct json_object *request = json_object_array_get_idx(batch, i);
        struct json_object *id_obj = NULL;
        BUFFER *response = mcp_jsonrpc_validate_request(request, &id_obj);

        if (!response) {
            bool has_id = json_object_object_get_ex(request, "id", NULL);

            if (!exhausted) {
                response = mcp_jsonrpc_process_single_request(mcpc, request, NULL);
                size_t response_len = response ? buffer_strlen(response) : 0;

                if (used + response_len > max_bytes) {
                    buffer_free(response);
                    response = NULL;
                    exhausted = true;
                }
                else {
                    used += response_len;
                    exhausted = used >= max_bytes;
                }
            }

            // not run, or its response did not fit in the budget
            if (!response && has_id)
                response = mcp_jsonrpc_build_error_payload(id_obj, -32001, "Batch response too large", NULL, 0);
        }

        if (response) {
            emit(response, data);
            buffer_free(response);
        }
    }

    return NULL;
}

BUFFER *mcp_jsonrpc_process_batch(MCP_CLIENT *mcpc, struct json_object *batch,
                                  mcp_jsonrpc_batch_emit_t emit, void *data) {
    return mcp_jsonrpc_process_batch_limited(mcpc, batch, emit, data,
                                             MCP_JSONRPC_BATCH_MAX_ENTRIES, MCP_JSONRPC_RESPONSE_MAX_BYTES);
}

// ----------------------------------------------------------------------------
// Batch limits: a batch with too many requests runs none of them, and once the responses reach
// the byte budget the remaining requests are not run. An "initialize" request is the probe:
// running it sets the client's protocol version.

static size_t mcp_jsonrpc_ut_emits = 0;

static void mcp_jsonrpc_ut_emit(BUFFER *response, void *data) {
    mcp_jsonrpc_ut_emits++;
    mcp_jsonrpc_batch_emit_array(response, data);
}

static struct json_object *mcp_jsonrpc_ut_parse(const char *text) {
    enum json_tokener_error jerr = json_tokener_success;
    return mcp_jsonrpc_parse_request(text, strlen(text), &jerr);
}

// the response of one request run alone on a fresh client
static void mcp_jsonrpc_ut_single(BUFFER *expected, const char *request_text) {
    MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, NULL);
    struct json_object *request = mcp_jsonrpc_ut_parse(request_text);
    BUFFER *response = mcp_jsonrpc_process_single_request(mcpc, request, NULL);
    mcp_jsonrpc_batch_emit_array(response, expected);
    buffer_free(response);
    json_object_put(request);
    mcp_free_client(mcpc);
}

static int mcp_jsonrpc_ut_check(const char *name, const char *batch_text, size_t max_entries, size_t max_bytes,
                                bool expect_invalid, size_t expect_emits, const char *expect_output,
                                bool expect_initialized) {
    MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, NULL);
    struct json_object *batch = mcp_jsonrpc_ut_parse(batch_text);
    BUFFER *out = buffer_create(1024, NULL);
    int errors = 0;

    mcp_jsonrpc_ut_emits = 0;
    BUFFER *invalid = mcp_jsonrpc_process_batch_limited(mcpc, batch, mcp_jsonrpc_ut_emit, out, max_entries, max_bytes);
    if (buffer_strlen(out))
        buffer_fast_strcat(out, "]", 1);

    if ((invalid != NULL) != expect_invalid) {
        fprintf(stderr, "  FAILED %s: batch %s rejected\n", name, invalid ? "was" : "was not");
        errors++;
    }
    if (mcp_jsonrpc_ut_emits != expect_emits) {
        fprintf(stderr, "  FAILED %s: %zu responses, expected %zu\n", name, mcp_jsonrpc_ut_emits, expect_emits);
        errors++;
    }
    if (expect_output && strcmp(buffer_tostring(out), expect_output) != 0) {
        fprintf(stderr, "  FAILED %s:\n    got      %s\n    expected %s\n", name, buffer_tostring(out), expect_output);
        errors++;
    }
    bool initialized = mcpc->protocol_version != MCP_PROTOCOL_VERSION_UNKNOWN;
    if (initialized != expect_initialized) {
        fprintf(stderr, "  FAILED %s: initialize %s\n", name, initialized ? "ran" : "did not run");
        errors++;
    }

    buffer_free(invalid);
    buffer_free(out);
    json_object_put(batch);
    mcp_free_client(mcpc);
    return errors;
}

#define UT_PING(id) "{\"jsonrpc\":\"2.0\",\"id\":" #id ",\"method\":\"ping\"}"
#define UT_INIT(id) "{\"jsonrpc\":\"2.0\",\"id\":" #id ",\"method\":\"initialize\",\"params\":{}}"

int mcp_jsonrpc_batch_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);
    int errors = 0;

    // positive control: initialize inside an accepted batch runs
    errors += mcp_jsonrpc_ut_check("control", "[" UT_INIT(1) "]", 64, 1024 * 1024, false, 1, NULL, true);

    // empty batch: one error, nothing runs
    errors += mcp_jsonrpc_ut_check("empty", "[]", 64, 1024 * 1024, true, 0, NULL, false);

    // exactly at the entry limit: all run; one more: none run
    {
        BUFFER *at_limit = buffer_create(1024, NULL);
        BUFFER *over_limit = buffer_create(1024, NULL);
        BUFFER *expected = buffer_create(1024, NULL);
        buffer_strcat(at_limit, "[" UT_INIT(0));
        mcp_jsonrpc_ut_single(expected, UT_INIT(0));
        for (size_t i = 1; i < 64; i++) {
            buffer_strcat(at_limit, "," UT_PING(7));
            mcp_jsonrpc_ut_single(expected, UT_PING(7));
        }
        buffer_strcat(over_limit, buffer_tostring(at_limit));
        buffer_strcat(at_limit, "]");
        buffer_strcat(over_limit, "," UT_PING(7) "]");
        buffer_fast_strcat(expected, "]", 1);

        MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, NULL);
        struct json_object *batch = mcp_jsonrpc_ut_parse(buffer_tostring(at_limit));
        BUFFER *out = buffer_create(1024, NULL);
        BUFFER *invalid = mcp_jsonrpc_process_batch(mcpc, batch, mcp_jsonrpc_batch_emit_array, out);
        buffer_fast_strcat(out, "]", 1);
        // the initialize response carries this agent's version, so compare with the same run alone
        if (invalid || strcmp(buffer_tostring(out), buffer_tostring(expected)) != 0) {
            fprintf(stderr, "  FAILED at-limit batch: unexpected output\n");
            errors++;
        }
        buffer_free(invalid);
        buffer_free(out);
        json_object_put(batch);
        mcp_free_client(mcpc);

        errors += mcp_jsonrpc_ut_check("over-limit", buffer_tostring(over_limit), MCP_JSONRPC_BATCH_MAX_ENTRIES,
                                       MCP_JSONRPC_RESPONSE_MAX_BYTES, true, 0, NULL, false);

        // the documented limits
        if (MCP_JSONRPC_BATCH_MAX_ENTRIES != 64 || MCP_JSONRPC_RESPONSE_MAX_BYTES != 16 * 1024 * 1024) {
            fprintf(stderr, "  FAILED limits: %zu requests, %zu bytes\n",
                    MCP_JSONRPC_BATCH_MAX_ENTRIES, MCP_JSONRPC_RESPONSE_MAX_BYTES);
            errors++;
        }
        buffer_free(at_limit);
        buffer_free(over_limit);
        buffer_free(expected);
    }

    // byte budget: two pings fit, the third does not and is replaced by an error, then nothing else
    // runs: requests with an id get the error, the notification gets nothing, malformed requests
    // still get their own error, and the initialize does not run.
    {
        BUFFER *ping = buffer_create(1024, NULL);
        mcp_jsonrpc_ut_single(ping, UT_PING(1));
        size_t ping_len = buffer_strlen(ping) - 1; // without the leading "["

        BUFFER *expected = buffer_create(1024, NULL);
        mcp_jsonrpc_ut_single(expected, UT_PING(1));
        mcp_jsonrpc_ut_single(expected, UT_PING(2));
        buffer_strcat(expected,
                      ",{\"jsonrpc\":\"2.0\",\"id\":3,\"error\":{\"code\":-32001,\"message\":\"Batch response too large\"}}"
                      ",{\"jsonrpc\":\"2.0\",\"id\":5,\"error\":{\"code\":-32600,\"message\":\"Invalid or missing jsonrpc version\"}}"
                      ",{\"jsonrpc\":\"2.0\",\"id\":6,\"error\":{\"code\":-32001,\"message\":\"Batch response too large\"}}"
                      ",{\"jsonrpc\":\"2.0\",\"id\":null,\"error\":{\"code\":-32600,\"message\":\"Invalid request\"}}"
                      "]");

        errors += mcp_jsonrpc_ut_check(
            "budget",
            "[" UT_PING(1) "," UT_PING(2) "," UT_PING(3) ","
            "{\"jsonrpc\":\"2.0\",\"method\":\"ping\"},"
            "{\"id\":5,\"method\":\"ping\"}," UT_INIT(6) ",7]",
            64, 2 * ping_len + ping_len / 2, false, 6, buffer_tostring(expected), false);

        // responses exactly at the budget: the next request does not run
        BUFFER *exact = buffer_create(1024, NULL);
        mcp_jsonrpc_ut_single(exact, UT_PING(1));
        mcp_jsonrpc_ut_single(exact, UT_PING(2));
        buffer_strcat(exact,
                      ",{\"jsonrpc\":\"2.0\",\"id\":3,\"error\":{\"code\":-32001,\"message\":\"Batch response too large\"}}]");
        errors += mcp_jsonrpc_ut_check("budget-exact", "[" UT_PING(1) "," UT_PING(2) "," UT_INIT(3) "]",
                                       64, 2 * ping_len, false, 3, buffer_tostring(exact), false);
        buffer_free(exact);

        buffer_free(ping);
        buffer_free(expected);
    }

    // an id that is not a string, a number or null: the request does not run
    errors += mcp_jsonrpc_ut_check(
        "bad-id", "[{\"jsonrpc\":\"2.0\",\"id\":{},\"method\":\"initialize\",\"params\":{}}]",
        64, 1024 * 1024, false, 1,
        "[{\"jsonrpc\":\"2.0\",\"id\":null,\"error\":{\"code\":-32600,\"message\":\"Invalid id\"}}]", false);

    // notifications only: they run and nothing is returned
    errors += mcp_jsonrpc_ut_check("notifications", "[{\"jsonrpc\":\"2.0\",\"method\":\"ping\"}]",
                                   64, 1024 * 1024, false, 0, NULL, false);

    if (errors)
        fprintf(stderr, "%s() FAILED with %d error(s)\n", __FUNCTION__, errors);
    else
        fprintf(stderr, "%s() passed\n", __FUNCTION__);

    return errors ? 1 : 0;
}
