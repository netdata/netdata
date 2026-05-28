// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// "JSON parser failed: " prefix (20 chars) + json-c error string (< 60 chars) + NUL
#define JSON_PARSER_ERROR_MSG_MAX 256
#define JSON_PARSER_ERROR_PREFIX "JSON parser failed: "
#define JSON_PARSER_ERROR_TRUNCATION_SUFFIX "..."
#define JSON_PARSER_UNKNOWN_ERROR "unknown error"

static void json_parser_format_error(char *dst, size_t dst_size, const char *error_msg, size_t error_len) {
    if(unlikely(!dst_size))
        return;

    if(!error_msg || !*error_msg) {
        error_msg = JSON_PARSER_UNKNOWN_ERROR;
        error_len = sizeof(JSON_PARSER_UNKNOWN_ERROR) - 1;
    }

    const size_t prefix_len = sizeof(JSON_PARSER_ERROR_PREFIX) - 1;
    size_t available = dst_size - 1;
    if(unlikely(available <= prefix_len)) {
        dst[0] = '\0';
        return;
    }

    available -= prefix_len;

    bool truncated = error_len > available;
    if(truncated) {
        size_t suffix_len = sizeof(JSON_PARSER_ERROR_TRUNCATION_SUFFIX) - 1;
        if(available > suffix_len)
            available -= suffix_len;
    }

    snprintfz(dst, dst_size, "%s%.*s%s",
              JSON_PARSER_ERROR_PREFIX,
              (int)available,
              error_msg,
              truncated ? JSON_PARSER_ERROR_TRUNCATION_SUFFIX : "");
}

int rrd_call_function_error(BUFFER *wb, const char *msg, int code) {
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_int64(wb, "status", code);
    buffer_json_member_add_string(wb, "errorMessage", msg);
    buffer_json_finalize(wb);
    wb->date = now_realtime_sec();
    wb->expires = wb->date + 1;
    wb->response_code = code;
    return code;
}

struct json_object *json_parse_function_payload_or_error(BUFFER *output, BUFFER *payload, int *code, json_parse_function_payload_t cb, void *cb_data) {
    if(!payload || !buffer_strlen(payload)) {
        *code = rrd_call_function_error(output, "No payload given, but a payload is required for this feature.", HTTP_RESP_BAD_REQUEST);
        return NULL;
    }

    struct json_tokener *tokener = json_tokener_new();
    if (!tokener) {
        *code = rrd_call_function_error(output, "Failed to initialize json parser.", HTTP_RESP_INTERNAL_SERVER_ERROR);
        return NULL;
    }

    struct json_object *jobj = json_tokener_parse_ex(tokener, buffer_tostring(payload), (int)buffer_strlen(payload));
    if (json_tokener_get_error(tokener) != json_tokener_success) {
        const char *error_msg = json_tokener_error_desc(json_tokener_get_error(tokener));
        char tmp[JSON_PARSER_ERROR_MSG_MAX];
        json_parser_format_error(tmp, sizeof(tmp), error_msg, error_msg ? strnlen(error_msg, JSON_PARSER_ERROR_MSG_MAX) : 0);
        json_tokener_free(tokener);
        *code = rrd_call_function_error(output, tmp, HTTP_RESP_INTERNAL_SERVER_ERROR);
        return NULL;
    }
    json_tokener_free(tokener);

    CLEAN_BUFFER *error = buffer_create(0, NULL);
    if(!cb(jobj, cb_data, error)) {
        char tmp[JSON_PARSER_ERROR_MSG_MAX];
        json_parser_format_error(tmp, sizeof(tmp), buffer_tostring(error), buffer_strlen(error));
        *code = rrd_call_function_error(output, tmp, HTTP_RESP_BAD_REQUEST);
        json_object_put(jobj);
        return NULL;
    }

    *code = HTTP_RESP_OK;

    return jobj;
}

int json_parse_payload_or_error(BUFFER *payload, BUFFER *error, json_parse_function_payload_t cb, void *cb_data) {
    if(!payload || !buffer_strlen(payload)) {
        buffer_strcat(error, "No payload given, but a payload is required for this feature.");
        return HTTP_RESP_BAD_REQUEST;
    }

    struct json_tokener *tokener = json_tokener_new();
    if (!tokener) {
        buffer_strcat(error, "Failed to initialize json parser.");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    struct json_object *jobj = json_tokener_parse_ex(tokener, buffer_tostring(payload), (int)buffer_strlen(payload));
    if (json_tokener_get_error(tokener) != json_tokener_success) {
        const char *error_msg = json_tokener_error_desc(json_tokener_get_error(tokener));
        char tmp[JSON_PARSER_ERROR_MSG_MAX];
        json_parser_format_error(tmp, sizeof(tmp), error_msg, error_msg ? strnlen(error_msg, JSON_PARSER_ERROR_MSG_MAX) : 0);
        json_tokener_free(tokener);
        buffer_strcat(error, tmp);
        return HTTP_RESP_BAD_REQUEST;
    }
    json_tokener_free(tokener);

    if(!cb(jobj, cb_data, error)) {
        if(!buffer_strlen(error))
            buffer_strcat(error, "Unknown error during parsing");
        json_object_put(jobj);
        return HTTP_RESP_BAD_REQUEST;
    }

    json_object_put(jobj);
    return HTTP_RESP_OK;
}
