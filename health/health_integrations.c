// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

// --------------------------------------------------------------------------------------------------------------------

#define ENUM_STR_MAP_DEFINE(type)   \
static struct  {                    \
    type id;                        \
    const char *name;               \
} type ## _names[]

#define ENUM_STR_DEFINE_FUNCTIONS_EXTERN(type)                                                                         \
    type type ## _2id(const char *str);                                                                                \
    const char *type##_2str(type id);

#define ENUM_STR_DEFINE_FUNCTIONS(type, def, def_str)                                                                  \
    type type##_2id(const char *str)                                                                                   \
    {                                                                                                                  \
        if (!str || !*str)                                                                                             \
            return def;                                                                                                \
                                                                                                                       \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                              \
            if (strcmp(type ## _names[i].name, str) == 0)                                                              \
                return type ## _names[i].id;                                                                           \
        }                                                                                                              \
                                                                                                                       \
        return def;                                                                                                    \
    }                                                                                                                  \
                                                                                                                       \
    const char *type##_2str(type id)                                                                                   \
    {                                                                                                                  \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                              \
            if (id == type ## _names[i].id)                                                                            \
                return type ## _names[i].name;                                                                         \
        }                                                                                                              \
                                                                                                                       \
        return def_str;                                                                                                \
    }

// --------------------------------------------------------------------------------------------------------------------

typedef enum __attribute__((packed)) {
    NOTIF_CURL_NONE = 0,
    NOTIF_CURL_GET,
    NOTIF_CURL_POST,
    NOTIF_CURL_PUT,
} NOTIF_CURL_METHOD;

ENUM_STR_MAP_DEFINE(NOTIF_CURL_METHOD) = {
    { NOTIF_CURL_GET, "GET" },
    { NOTIF_CURL_POST, "POST" },
    { NOTIF_CURL_PUT, "PUT" },

    // terminator
    { 0, NULL }
};

ENUM_STR_DEFINE_FUNCTIONS(NOTIF_CURL_METHOD, NOTIF_CURL_NONE, NULL)

// --------------------------------------------------------------------------------------------------------------------

typedef enum __attribute__((packed)) {
    NOTIF_CURL_CT_NONE = 0,
    NOTIF_CURL_FORM_DATA,
    NOTIF_CURL_APPLICATION_JSON,
} NOTIF_CURL_CONTENT_TYPE;

ENUM_STR_MAP_DEFINE(NOTIF_CURL_CONTENT_TYPE) = {
    { NOTIF_CURL_FORM_DATA, "Form Data" },
    { NOTIF_CURL_APPLICATION_JSON, "application/json" },

    // terminator
    { 0, NULL }
};

ENUM_STR_DEFINE_FUNCTIONS(NOTIF_CURL_CONTENT_TYPE, NOTIF_CURL_CT_NONE, NULL)

// --------------------------------------------------------------------------------------------------------------------

typedef struct notif_variable {
    STRING *name;
    STRING *help;

    struct notif_variable *prev, *next;
} NOTIF_VARIABLE;

static void NOTIF_VARIABLE_free(NOTIF_VARIABLE *nv) {
    string_freez(nv->name);
    string_freez(nv->help);
    freez(nv);
}

static void NOTIF_VARIABLE_to_json(BUFFER *wb, const char *key, NOTIF_VARIABLE *nv) {
    if(key)
        buffer_json_member_add_object(wb, key);
    else
        buffer_json_add_array_item_object(wb);

    buffer_json_member_add_string(wb, "name", string2str(nv->name));
    buffer_json_member_add_string(wb, "help", string2str(nv->help));

    buffer_json_object_close(wb);
}

static bool NOTIF_VARIABLE_from_json(json_object *jobj, const char *path, NOTIF_VARIABLE **base, BUFFER *error) {
    NOTIF_VARIABLE *nv = callocz(1, sizeof(*nv));
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*base, nv, prev, next); // link first, so the caller will cleanup
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "name", nv->name, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "help", nv->help, error, true);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

typedef struct notif_name_value {
    STRING *name;
    STRING *value;

    struct notif_name_value *prev, *next;
} NOTIF_NAME_VALUE;

static void NOTIF_NAME_VALUE_free(NOTIF_NAME_VALUE *nv) {
    string_freez(nv->name);
    string_freez(nv->value);
    freez(nv);
}

static void NOTIF_NAME_VALUE_to_json(BUFFER *wb, const char *key, NOTIF_NAME_VALUE *nnv) {
    if(key)
        buffer_json_member_add_object(wb, key);
    else
        buffer_json_add_array_item_object(wb);

    buffer_json_member_add_string(wb, "name", string2str(nnv->name));
    buffer_json_member_add_string(wb, "value", string2str(nnv->value));

    buffer_json_object_close(wb);
}

static bool NOTIF_NAME_VALUE_from_json(json_object *jobj, const char *path, NOTIF_NAME_VALUE **base, BUFFER *error) {
    NOTIF_NAME_VALUE *nnv = callocz(1, sizeof(*nnv));
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*base, nnv, prev, next); // link first, so the caller will cleanup
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "name", nnv->name, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "value", nnv->value, error, true);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

typedef struct notif_severity_map {
    STRING *name;

    STRING *clear;
    STRING *warning;
    STRING *critical;

    struct notif_severity_map *prev, *next;
} NOTIF_SEVERITY_MAP;

static void NOTIF_SEVERITY_MAP_free(NOTIF_SEVERITY_MAP *nv) {
    string_freez(nv->name);
    string_freez(nv->clear);
    string_freez(nv->warning);
    string_freez(nv->critical);
    freez(nv);
}

static void NOTIF_SEVERITY_MAP_to_json(BUFFER *wb, const char *key, NOTIF_SEVERITY_MAP *nsm) {
    if(key)
        buffer_json_member_add_object(wb, key);
    else
        buffer_json_add_array_item_object(wb);

    buffer_json_member_add_string(wb, "name", string2str(nsm->name));
    buffer_json_member_add_string(wb, "clear", string2str(nsm->clear));
    buffer_json_member_add_string(wb, "warning", string2str(nsm->warning));
    buffer_json_member_add_string(wb, "critical", string2str(nsm->critical));

    buffer_json_object_close(wb);
}

static bool NOTIF_SEVERITY_MAP_from_json(json_object *jobj, const char *path, NOTIF_SEVERITY_MAP **base, BUFFER *error) {
    NOTIF_SEVERITY_MAP *nsm = callocz(1, sizeof(*nsm));
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(*base, nsm, prev, next); // link first, so the caller will cleanup
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "name", nsm->name, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "clear", nsm->clear, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "warning", nsm->warning, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "critical", nsm->critical, error, true);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

typedef struct notif_curl {
    STRING *name;
    STRING *url;
    STRING *docs;
    NOTIF_VARIABLE *user_config;
    NOTIF_NAME_VALUE *headers;
    NOTIF_NAME_VALUE *form_data;
    NOTIF_SEVERITY_MAP *severity_map;
    NOTIF_CURL_METHOD method;
    NOTIF_CURL_CONTENT_TYPE content_type;
    BUFFER *payload;
} NOTIF_CURL;

static void NOTIF_CURL_cleanup(NOTIF_CURL *nc) {
    string_freez(nc->name);
    string_freez(nc->url);
    string_freez(nc->docs);

    while(nc->user_config) {
        NOTIF_VARIABLE *nv = nc->user_config;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(nc->user_config, nv, prev, next);
        NOTIF_VARIABLE_free(nv);
    }

    while(nc->headers) {
        NOTIF_NAME_VALUE *nnv = nc->headers;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(nc->headers, nnv, prev, next);
        NOTIF_NAME_VALUE_free(nnv);
    }

    while(nc->form_data) {
        NOTIF_NAME_VALUE *nnv = nc->form_data;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(nc->form_data, nnv, prev, next);
        NOTIF_NAME_VALUE_free(nnv);
    }

    while(nc->severity_map) {
        NOTIF_SEVERITY_MAP *nsm = nc->severity_map;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(nc->severity_map, nsm, prev, next);
        NOTIF_SEVERITY_MAP_free(nsm);
    }

    buffer_free(nc->payload);

    memset(nc, 0, sizeof(*nc));
}

static void NOTIF_CURL_free(NOTIF_CURL *nc) {
    NOTIF_CURL_cleanup(nc);
    freez(nc);
}

static void NOTIF_CURL_to_json(BUFFER *wb, NOTIF_CURL *nc) {
    buffer_flush(wb);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "format_version", 1);
    buffer_json_member_add_string(wb, "name", string2str(nc->name));
    buffer_json_member_add_string(wb, "url", string2str(nc->url));
    buffer_json_member_add_string(wb, "docs", string2str(nc->docs));
    buffer_json_member_add_string(wb, "payload", nc->payload ? buffer_tostring(nc->payload) : "");
    buffer_json_member_add_string(wb, "method", NOTIF_CURL_METHOD_2str(nc->method));
    buffer_json_member_add_string(wb, "content_type", NOTIF_CURL_CONTENT_TYPE_2str(nc->content_type));

    buffer_json_member_add_array(wb, "variables");
    for(NOTIF_VARIABLE *nv = nc->user_config; nv ;nv = nv->next)
        NOTIF_VARIABLE_to_json(wb, NULL, nv);
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "headers");
    for(NOTIF_NAME_VALUE *nnv = nc->headers; nnv ;nnv = nnv->next)
        NOTIF_NAME_VALUE_to_json(wb, NULL, nnv);
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "form_data");
    for(NOTIF_NAME_VALUE *nnv = nc->form_data; nnv ;nnv = nnv->next)
        NOTIF_NAME_VALUE_to_json(wb, NULL, nnv);
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "severity_map");
    for(NOTIF_SEVERITY_MAP *nsm = nc->severity_map; nsm ;nsm = nsm->next)
        NOTIF_SEVERITY_MAP_to_json(wb, NULL, nsm);
    buffer_json_array_close(wb);

    buffer_json_finalize(wb);
}

static bool NOTIF_CURL_from_json2(json_object *jobj, const char *path, NOTIF_CURL *nc, BUFFER *error, const char *name) {
    int64_t version;
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "format_version", version, error);

    if(version != 1) {
        buffer_sprintf(error, "unsupported document version");
        return false;
    }

    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "name", nc->name, error, !name || !*name);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "url", nc->url, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "docs", nc->docs, error, true);
    JSONC_PARSE_TXT2BUFFER_OR_ERROR_AND_RETURN(jobj, path, "payload", nc->payload, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "method", NOTIF_CURL_METHOD_2id, nc->method, error);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "content_type", NOTIF_CURL_CONTENT_TYPE_2id, nc->content_type, error);

    json_object *jarr;

    if (json_object_object_get_ex(jobj, "variables", &jarr)) {
        for(size_t i = 0, len = json_object_array_length(jarr); i < len ;i++)
            if(!NOTIF_VARIABLE_from_json(jarr, path, &nc->user_config, error))
                return false;
    }

    if (json_object_object_get_ex(jobj, "headers", &jarr)) {
        for(size_t i = 0, len = json_object_array_length(jarr); i < len ;i++)
            if(!NOTIF_NAME_VALUE_from_json(jarr, path, &nc->headers, error))
                return false;
    }

    if (json_object_object_get_ex(jobj, "form_data", &jarr)) {
        for(size_t i = 0, len = json_object_array_length(jarr); i < len ;i++)
            if(!NOTIF_NAME_VALUE_from_json(jarr, path, &nc->form_data, error))
                return false;
    }

    if (json_object_object_get_ex(jobj, "severity_map", &jarr)) {
        for(size_t i = 0, len = json_object_array_length(jarr); i < len ;i++)
            if(!NOTIF_SEVERITY_MAP_from_json(jarr, path, &nc->severity_map, error))
                return false;
    }

    return true;
}

NOTIF_CURL *NOTIF_CURL_from_json(const char *payload, size_t payload_len, BUFFER *error, const char *name) {
    NOTIF_CURL *nc = callocz(1, sizeof(*nc));

    CLEAN_JSON_OBJECT *jobj = NULL;

    struct json_tokener *tokener = json_tokener_new();
    if (!tokener) {
        buffer_sprintf(error, "failed to allocate memory for json tokener");
        goto cleanup;
    }

    jobj = json_tokener_parse_ex(tokener, payload, (int)payload_len);
    if (json_tokener_get_error(tokener) != json_tokener_success) {
        const char *error_msg = json_tokener_error_desc(json_tokener_get_error(tokener));
        buffer_sprintf(error, "failed to parse json payload: %s", error_msg);
        json_tokener_free(tokener);
        goto cleanup;
    }
    json_tokener_free(tokener);

    if(!NOTIF_CURL_from_json2(jobj, "", nc, error, name))
        goto cleanup;

    if(!nc->name && name)
        nc->name = string_strdupz(name);

    return nc;

cleanup:
    NOTIF_CURL_free(nc);
    return NULL;

}

// --------------------------------------------------------------------------------------------------------------------

static struct {
    DICTIONARY *integrations;
} notif_curl_globals;

// --------------------------------------------------------------------------------------------------------------------

int dyncfg_notif_curl_cb(const char *transaction, const char *id, DYNCFG_CMDS cmd, const char *add_name,
                   BUFFER *payload, usec_t *stop_monotonic_ut, bool *cancelled, BUFFER *result,
                   HTTP_ACCESS access, const char *source, void *data) {

    int rc = dyncfg_default_response(result, HTTP_RESP_INTERNAL_SERVER_ERROR, "not implemented yet");

    return rc;
}

// --------------------------------------------------------------------------------------------------------------------

static bool notif_curl_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    NOTIF_CURL *nc = old_value, *nnc = new_value;
    SWAP(*nc, *nnc);
    NOTIF_CURL_cleanup(nnc);
    return true;
}

static void notif_curl_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    NOTIF_CURL_cleanup(value);
}

void notif_curl_init(void) {
    notif_curl_globals.integrations = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(NOTIF_CURL));
    dictionary_register_conflict_callback(notif_curl_globals.integrations, notif_curl_conflict_cb, NULL);
    dictionary_register_delete_callback(notif_curl_globals.integrations, notif_curl_delete_cb, NULL);

    dyncfg_add(localhost,
               "health:notification:integration",
               "/health/notifications/integrations",
               DYNCFG_STATUS_ACCEPTED, DYNCFG_TYPE_TEMPLATE, DYNCFG_SOURCE_TYPE_INTERNAL, "",
               DYNCFG_CMD_ADD | DYNCFG_CMD_SCHEMA,
               HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG,
               HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG,
               dyncfg_notif_curl_cb, NULL);
}

// --------------------------------------------------------------------------------------------------------------------

