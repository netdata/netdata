// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_TYPE type;
    const char *name;
} dyncfg_types[] = {
    { .type = DYNCFG_TYPE_SINGLE, .name = "single" },
    { .type = DYNCFG_TYPE_TEMPLATE, .name = "template" },
    { .type = DYNCFG_TYPE_JOB, .name = "job" },
};

DYNCFG_TYPE dyncfg_type2id(const char *type) {
    if(!type || !*type)
        return DYNCFG_TYPE_SINGLE;

    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_types[i].name, type) == 0)
            return dyncfg_types[i].type;
    }

    return DYNCFG_TYPE_SINGLE;
}

const char *dyncfg_id2type(DYNCFG_TYPE type) {
    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(type == dyncfg_types[i].type)
            return dyncfg_types[i].name;
    }

    return "single";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_SOURCE_TYPE source_type;
    const char *name;
} dyncfg_source_types[] = {
    { .source_type = DYNCFG_SOURCE_TYPE_INTERNAL, .name = "internal" },
    { .source_type = DYNCFG_SOURCE_TYPE_STOCK, .name = "stock" },
    { .source_type = DYNCFG_SOURCE_TYPE_USER, .name = "user" },
    { .source_type = DYNCFG_SOURCE_TYPE_DYNCFG, .name = "dyncfg" },
    { .source_type = DYNCFG_SOURCE_TYPE_DISCOVERED, .name = "discovered" },
};

DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type) {
    if(!source_type || !*source_type)
        return DYNCFG_SOURCE_TYPE_INTERNAL;

    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_source_types[i].name, source_type) == 0)
            return dyncfg_source_types[i].source_type;
    }

    return DYNCFG_SOURCE_TYPE_INTERNAL;
}

const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type) {
    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(source_type == dyncfg_source_types[i].source_type)
            return dyncfg_source_types[i].name;
    }

    return "internal";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_STATUS status;
    const char *name;
} dyncfg_statuses[] = {
    { .status = DYNCFG_STATUS_NONE, .name = "none" },
    { .status = DYNCFG_STATUS_ACCEPTED, .name = "accepted" },
    { .status = DYNCFG_STATUS_RUNNING, .name = "running" },
    { .status = DYNCFG_STATUS_FAILED, .name = "failed" },
    { .status = DYNCFG_STATUS_DISABLED, .name = "disabled" },
    { .status = DYNCFG_STATUS_ORPHAN, .name = "orphan" },
    { .status = DYNCFG_STATUS_INCOMPLETE, .name = "incomplete" },
};

DYNCFG_STATUS dyncfg_status2id(const char *status) {
    if(!status || !*status)
        return DYNCFG_STATUS_NONE;

    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_statuses[i].name, status) == 0)
            return dyncfg_statuses[i].status;
    }

    return DYNCFG_STATUS_NONE;
}

const char *dyncfg_id2status(DYNCFG_STATUS status) {
    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(status == dyncfg_statuses[i].status)
            return dyncfg_statuses[i].name;
    }

    return "none";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_CMDS cmd;
    const char *name;
} cmd_map[] = {
    { .cmd = DYNCFG_CMD_GET, .name = "get" },
    { .cmd = DYNCFG_CMD_SCHEMA, .name = "schema" },
    { .cmd = DYNCFG_CMD_UPDATE, .name = "update" },
    { .cmd = DYNCFG_CMD_ADD, .name = "add" },
    { .cmd = DYNCFG_CMD_TEST, .name = "test" },
    { .cmd = DYNCFG_CMD_REMOVE, .name = "remove" },
    { .cmd = DYNCFG_CMD_ENABLE, .name = "enable" },
    { .cmd = DYNCFG_CMD_DISABLE, .name = "disable" },
    { .cmd = DYNCFG_CMD_RESTART, .name = "restart" },
    { .cmd = DYNCFG_CMD_USERCONFIG, .name = "userconfig" },
};

const char *dyncfg_id2cmd_one(DYNCFG_CMDS cmd) {
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmd == cmd_map[i].cmd)
            return cmd_map[i].name;
    }

    return NULL;
}

DYNCFG_CMDS dyncfg_cmds2id(const char *cmds) {
    if(!cmds || !*cmds)
        return DYNCFG_CMD_NONE;

    DYNCFG_CMDS result = DYNCFG_CMD_NONE;
    const char *p = cmds;
    size_t len, i;

    while (*p) {
        // Skip any leading spaces
        while (*p == ' ') p++;

        // Find the end of the current word
        const char *end = p;
        while (*end && *end != ' ') end++;
        len = end - p;

        // Compare with known commands
        for (i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
            if (strncmp(p, cmd_map[i].name, len) == 0 && cmd_map[i].name[len] == '\0') {
                result |= cmd_map[i].cmd;
                break;
            }
        }

        // Move to the next word
        p = end;
    }

    return result;
}

void dyncfg_cmds2fp(DYNCFG_CMDS cmds, FILE *fp) {
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd)
            fprintf(fp, "%s ", cmd_map[i].name);
    }
}

void dyncfg_cmds2json_array(DYNCFG_CMDS cmds, const char *key, BUFFER *wb) {
    buffer_json_member_add_array(wb, key);
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd)
            buffer_json_add_array_item_string(wb, cmd_map[i].name);
    }
    buffer_json_array_close(wb);
}

void dyncfg_cmds2buffer(DYNCFG_CMDS cmds, BUFFER *wb) {
    size_t added = 0;
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd) {
            if(added)
                buffer_fast_strcat(wb, " ", 1);

            buffer_strcat(wb, cmd_map[i].name);
            added++;
        }
    }
}

// ----------------------------------------------------------------------------

bool dyncfg_is_valid_id(const char *id) {
    const char *s = id;

    while(*s) {
        if(isspace((uint8_t)*s) || *s == '\'') return false;
        s++;
    }

    return true;
}

static inline bool is_forbidden_filename_char(char c) {
    if(isspace((uint8_t)c) || !isprint((uint8_t)c))
        return true;

    switch(c) {
        case '`': // good not to have this in filenames
        case '$': // good not to have this in filenames
        case '/': // unix does not support this
        case ':': // windows does not support this
        case '|': // windows does not support this
            return true;

        default:
            return false;
    }
}

char *dyncfg_escape_id_for_filename(const char *id) {
    if (id == NULL) return NULL;

    // Allocate memory for the worst case, where every character is escaped.
    char *escaped = mallocz(strlen(id) * 3 + 1); // Each char can become '%XX', plus '\0'
    if (!escaped) return NULL;

    const char *src = id;
    char *dest = escaped;

    while (*src) {
        if (is_forbidden_filename_char(*src)) {
            sprintf(dest, "%%%02X", (unsigned char)*src);
            dest += 3;
        } else {
            *dest++ = *src;
        }
        src++;
    }

    *dest = '\0';
    return escaped;
}

// ----------------------------------------------------------------------------

int dyncfg_default_response(BUFFER *wb, int code, const char *msg) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_realtime_sec();

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "status", code);
    buffer_json_member_add_string(wb, "message", msg);
    buffer_json_finalize(wb);

    return code;
}

int dyncfg_node_find_and_call(DICTIONARY *dyncfg_nodes, const char *transaction, const char *function,
                              usec_t *stop_monotonic_ut, bool *cancelled,
                              BUFFER *payload, HTTP_ACCESS access, const char *source, BUFFER *result) {
    if(!function || !*function)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "command received is empty");

    char buf[strlen(function) + 1];
    memcpy(buf, function, sizeof(buf));

    char *words[MAX_FUNCTION_PARAMETERS];    // an array of pointers for the words in this line
    size_t num_words = quoted_strings_splitter_whitespace(buf, words, MAX_FUNCTION_PARAMETERS);

    const char *id = get_word(words, num_words, 1);
    const char *action = get_word(words, num_words, 2);
    const char *add_name = get_word(words, num_words, 3);

    if(!id || !*id)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "dyncfg node: id is missing from the request");

    if(!action || !*action)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "dyncfg node: action is missing from the request");

    DYNCFG_CMDS cmd = dyncfg_cmds2id(action);
    if(cmd == DYNCFG_CMD_NONE)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "dyncfg node: action given in request is unknown");

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dyncfg_nodes, id);
    if(!item)
        return dyncfg_default_response(result, HTTP_RESP_NOT_FOUND, "dyncfg node: id is not found");

    struct dyncfg_node *df = dictionary_acquired_item_value(item);

    buffer_flush(result);
    result->content_type = CT_APPLICATION_JSON;

    int code = df->cb(transaction, id, cmd, add_name, payload, stop_monotonic_ut, cancelled, result, access, source, df->data);

    if(!result->expires)
        result->expires = now_realtime_sec();

    if(!buffer_tostring(result))
        dyncfg_default_response(result, code, "");

    dictionary_acquired_item_release(dyncfg_nodes, item);

    return code;
}
