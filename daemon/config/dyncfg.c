// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg.h"

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
    { .source_type = DYNCFG_SOURCE_TYPE_STOCK, .name = "stock" },
    { .source_type = DYNCFG_SOURCE_TYPE_USER, .name = "user" },
    { .source_type = DYNCFG_SOURCE_TYPE_DYNCFG, .name = "dyncfg" },
    { .source_type = DYNCFG_SOURCE_TYPE_DISCOVERY, .name = "discovered" },
};

DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type) {
    if(!source_type || !*source_type)
        return DYNCFG_SOURCE_TYPE_STOCK;

    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_source_types[i].name, source_type) == 0)
            return dyncfg_source_types[i].source_type;
    }

    return DYNCFG_SOURCE_TYPE_STOCK;
}

const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type) {
    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(source_type == dyncfg_source_types[i].source_type)
            return dyncfg_source_types[i].name;
    }

    return "stock";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_STATUS status;
    const char *name;
} dyncfg_statuses[] = {
    { .status = DYNCFG_STATUS_OK, .name = "ok" },
    { .status = DYNCFG_STATUS_DISABLED, .name = "disabled" },
    { .status = DYNCFG_STATUS_ORPHAN, .name = "orphan" },
    { .status = DYNCFG_STATUS_REJECTED, .name = "rejected" },
};

DYNCFG_STATUS dyncfg_status2id(const char *status) {
    if(!status || !*status)
        return DYNCFG_STATUS_OK;

    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_statuses[i].name, status) == 0)
            return dyncfg_statuses[i].status;
    }

    return DYNCFG_STATUS_OK;
}

const char *dyncfg_id2status(DYNCFG_STATUS status) {
    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(status == dyncfg_statuses[i].status)
            return dyncfg_statuses[i].name;
    }

    return "ok";
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
    { .cmd = DYNCFG_CMD_REMOVE, .name = "remove" },
    { .cmd = DYNCFG_CMD_ENABLE, .name = "enable" },
    { .cmd = DYNCFG_CMD_DISABLE, .name = "disable" },
    { .cmd = DYNCFG_CMD_RESTART, .name = "restart" }
};

DYNCFG_CMDS dyncfg_cmds2id(const char *cmds) {
    DYNCFG_CMDS result = DYNCFG_CMD_NONE;
    const char *p = cmds;
    int len, i;

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

// ----------------------------------------------------------------------------

typedef struct dyncfg {
    STRING *path;
    DYNCFG_STATUS status;
    DYNCFG_TYPE type;
    DYNCFG_SOURCE_TYPE source_type;
    DYNCFG_CMDS cmds;
    STRING *source;
    usec_t created_ut;
    usec_t modified_ut;
    bool user_disabled;
    bool restart_required;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;
} DYNCFG;

struct {
    DICTIONARY *nodes;
} dyncfg_globals = { 0 };

static void dyncfg_cleanup(DYNCFG *v) {
    string_freez(v->path);
    v->path = NULL;

    string_freez(v->source);
    v->source = NULL;
}

static void dyncfg_normalize(DYNCFG *v) {
    usec_t now_ut = now_realtime_usec();

    if(!v->created_ut)
        v->created_ut = now_ut;

    if(!v->modified_ut)
        v->modified_ut = now_ut;
}

static void dyncfg_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *v = value;
    dyncfg_cleanup(v);
}

static void dyncfg_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *v = value;
    dyncfg_normalize(v);
}

static bool dyncfg_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    DYNCFG *v = old_value;
    DYNCFG *nv = new_value;

    size_t changes = 0;

    dyncfg_normalize(nv);

    if(v->path != nv->path) {
        STRING *old = v->path;
        v->path = nv->path;
        nv->path = NULL;
        string_freez(old);
        changes++;
    }

    if(v->status != nv->status) {
        v->status = nv->status;
        changes++;
    }

    if(v->type != nv->type) {
        v->type = nv->type;
        changes++;
    }

    if(v->source_type != nv->source_type) {
        v->source_type = nv->source_type;
        changes++;
    }

    if(v->source != nv->source) {
        STRING *old = v->source;
        v->source = nv->source;
        nv->source = NULL;
        string_freez(old);
        changes++;
    }

    if(v->cmds != nv->cmds) {
        v->cmds = nv->cmds;
        changes++;
    }

    if(nv->created_ut < v->created_ut) {
        v->created_ut = nv->created_ut;
        changes++;
    }

    if(nv->modified_ut > v->modified_ut) {
        v->modified_ut = nv->modified_ut;
        changes++;
    }

    dyncfg_cleanup(nv);

    return changes > 0;
}

void dyncfg_init(void) {
    if(!dyncfg_globals.nodes) {
        dyncfg_globals.nodes = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(DYNCFG));
        dictionary_register_insert_callback(dyncfg_globals.nodes, dyncfg_insert_cb, NULL);
        dictionary_register_conflict_callback(dyncfg_globals.nodes, dyncfg_conflict_cb, NULL);
        dictionary_register_delete_callback(dyncfg_globals.nodes, dyncfg_delete_cb, NULL);
    }
}

// ----------------------------------------------------------------------------
// echo is the first config command we send to the plugin

struct dyncfg_echo {
    const DICTIONARY_ITEM *item;
    DYNCFG *df;
    BUFFER *wb;
};

void dyncfg_echo_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_echo *e = result_cb_data;

    if(code != HTTP_RESP_OK) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to send the first config cmd to '%s', with error code %d",
               dictionary_acquired_item_name(e->item), code);

        e->df->status = DYNCFG_STATUS_REJECTED;
    }
    else
        e->df->status = DYNCFG_STATUS_OK;

    buffer_free(e->wb);
    dictionary_acquired_item_release(dyncfg_globals.nodes, e->item);

    e->wb = NULL;
    e->df = NULL;
    e->item = NULL;
    freez(e);
}

// ----------------------------------------------------------------------------
// call is when we intercept the config function calls of the plugin

struct dyncfg_call {
    STRING *function;
    STRING *id;
    DYNCFG_CMDS cmd;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
};

void dyncfg_function_result_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_call *dc = result_cb_data;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, string2str(dc->id), string_strlen(dc->id));
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);

        if(code == HTTP_RESP_NOT_FOUND) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Plugin returned not found error to call '%s', marking it as rejected.", string2str(dc->function));
            df->status = DYNCFG_STATUS_REJECTED;
        }
        else if(code == HTTP_RESP_NOT_IMPLEMENTED) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "Plugin returned not supported error to call '%s', disabling this action.", string2str(dc->function));
            df->cmds &= ~dc->cmd;
        }
        else if(code == HTTP_RESP_ACCEPTED) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "Plugin returned 202 to call '%s', restart is required.", string2str(dc->function));
            df->cmds &= ~dc->cmd;
        }

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }

    if(dc->result_cb)
        dc->result_cb(wb, code, dc->result_cb_data);

    string_freez(dc->function);
    string_freez(dc->id);
    freez(dc);
}

static int dyncfg_function_execute_cb(uuid_t *transaction, BUFFER *result_body_wb,
                                 usec_t *stop_monotonic_ut, const char *function,
                                 void *execute_cb_data,
                                 rrd_function_result_callback_t result_cb, void *result_cb_data,
                                 rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                 rrd_function_is_cancelled_cb_t is_cancelled_cb,
                                 void *is_cancelled_cb_data,
                                 rrd_function_register_canceller_cb_t register_canceller_cb,
                                 void *register_canceller_cb_data,
                                 rrd_function_register_progresser_cb_t register_progresser_cb,
                                 void *register_progresser_cb_data) {

    DYNCFG_CMDS c = DYNCFG_CMD_NONE;
    const DICTIONARY_ITEM *item = NULL;
    if(strncmp(function, "config ", 7) == 0) {
        const char *id = &function[7];
        while(isspace(*id)) id++;
        const char *space = id;
        while(*space && !isspace(*space)) space++;
        size_t id_len = space - id;

        const char *cmd = space;
        while(isspace(*cmd)) cmd++;
        space = cmd;
        while(*space && !isspace(*space)) space++;
        size_t cmd_len = space - cmd;

        char cmd_copy[cmd_len + 1];
        strncpyz(cmd_copy, cmd, cmd_len);
        c = dyncfg_cmds2id(cmd_copy);

        item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, id, id_len);
    }

    if(!item) {
        rrd_call_function_error(result_body_wb, "not found", HTTP_RESP_NOT_FOUND);

        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_NOT_FOUND, result_cb_data);

        return HTTP_RESP_NOT_FOUND;
    }

    struct dyncfg_call *dc = callocz(1, sizeof(*dc));
    dc->function = string_strdupz(function);
    dc->id = string_strdupz(dictionary_acquired_item_name(item));
    dc->cmd = c;
    dc->result_cb = result_cb;
    dc->result_cb_data = result_cb_data;

    DYNCFG *df = dictionary_acquired_item_value(item);
    int rc = df->execute_cb(transaction, result_body_wb, stop_monotonic_ut, function, df->execute_cb_data,
                            dyncfg_function_result_cb, dc,
                            progress_cb, progress_cb_data,
                            is_cancelled_cb, is_cancelled_cb_data,
                            register_canceller_cb, register_canceller_cb_data,
                            register_progresser_cb, register_progresser_cb_data);

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return rc;
}

// ----------------------------------------------------------------------------

void dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {
    dyncfg_init();

    DYNCFG tmp = {
        .path = string_strdupz(path),
        .status = status,
        .type = type,
        .cmds = cmds,
        .source_type = source_type,
        .source = string_strdupz(source),
        .created_ut = created_ut,
        .modified_ut = modified_ut,
        .user_disabled = false,
        .restart_required = false,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
    };

    size_t id_len = strlen(id);
    const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item_advanced(dyncfg_globals.nodes, id, id_len, &tmp, sizeof(tmp), NULL);
    DYNCFG *df = dictionary_acquired_item_value(item);

    char name[id_len + 100];
    snprintfz(name, sizeof(name), "config %s", id);

    rrd_collector_started();
    rrd_function_add(host, NULL, name, 10, 100,
                     "Dynamic configuration function", "config",
                     HTTP_ACCESS_MEMBERS, sync, dyncfg_function_execute_cb, NULL);

    // send the first command to it
    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = item;
    e->df = df;

    snprintfz(name, sizeof(name), "config %s %s", id, df->user_disabled ? "disable" : "enable");

    e->wb = buffer_create(0, NULL);
    int rc = rrd_function_run(host, e->wb, 10,
                              HTTP_ACCESS_ADMINS, name, false, NULL,
                              dyncfg_echo_cb, e,
                              NULL, NULL,
                              NULL, NULL,
                              NULL);

    if(rc != HTTP_RESP_OK)
        dyncfg_echo_cb(e->wb, rc, e);
}
