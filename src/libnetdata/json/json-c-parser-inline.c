// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int rrd_call_function_error(BUFFER *wb, const char *msg, int code) {
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_int64(wb, "status", code);
    buffer_json_member_add_string(wb, "error_message", msg);
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
        char tmp[strlen(error_msg) + 100];
        snprintf(tmp, sizeof(tmp), "JSON parser failed: %s", error_msg);
        json_tokener_free(tokener);
        *code = rrd_call_function_error(output, tmp, HTTP_RESP_INTERNAL_SERVER_ERROR);
        return NULL;
    }
    json_tokener_free(tokener);

    CLEAN_BUFFER *error = buffer_create(0, NULL);
    if(!cb(jobj, cb_data, error)) {
        char tmp[buffer_strlen(error) + 100];
        snprintfz(tmp, sizeof(tmp), "JSON parser failed: %s", buffer_tostring(error));
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
        char tmp[strlen(error_msg) + 100];
        snprintf(tmp, sizeof(tmp), "JSON parser failed: %s", error_msg);
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
