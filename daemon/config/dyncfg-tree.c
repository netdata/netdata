// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

static int dyncfg_tree_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM *item1 = *(const DICTIONARY_ITEM **)a;
    const DICTIONARY_ITEM *item2 = *(const DICTIONARY_ITEM **)b;

    DYNCFG *df1 = dictionary_acquired_item_value(item1);
    DYNCFG *df2 = dictionary_acquired_item_value(item2);

    int rc = string_cmp(df1->path, df2->path);
    if(rc == 0)
        rc = strcmp(dictionary_acquired_item_name(item1), dictionary_acquired_item_name(item2));

    return rc;
}

static void dyncfg_to_json(DYNCFG *df, const char *id, BUFFER *wb) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "type", dyncfg_id2type(df->type));
        buffer_json_member_add_string(wb, "status", dyncfg_id2status(df->status));
        dyncfg_cmds2json_array(df->cmds, "cmds", wb);
        buffer_json_member_add_string(wb, "source_type", dyncfg_id2source_type(df->source_type));
        buffer_json_member_add_string(wb, "source", string2str(df->source));
        buffer_json_member_add_boolean(wb, "sync", df->sync);
        buffer_json_member_add_boolean(wb, "user_disabled", df->user_disabled);
        buffer_json_member_add_boolean(wb, "restart_required", df->restart_required);
        if(df->payload && buffer_strlen(df->payload)) {
            buffer_json_member_add_object(wb, "payload");
            buffer_json_member_add_boolean(wb, "content_type", content_type_id2string(df->payload->content_type));
            buffer_json_member_add_uint64(wb, "content_length", df->payload->len);
            buffer_json_object_close(wb);
        }
        buffer_json_member_add_uint64(wb, "saves", df->saves);
        buffer_json_member_add_uint64(wb, "created_ut", df->created_ut);
        buffer_json_member_add_uint64(wb, "modified_ut", df->modified_ut);
    }
    buffer_json_object_close(wb);
}

static void dyncfg_tree_for_host(RRDHOST *host, BUFFER *wb, const char *parent) {
    size_t entries = dictionary_entries(dyncfg_globals.nodes);
    size_t used = 0;
    const DICTIONARY_ITEM *items[entries];

    size_t parent_len = strlen(parent);
    DYNCFG *df;
    dfe_start_read(dyncfg_globals.nodes, df) {
        if(!df->host) {
            if(uuid_memcmp(&df->host_uuid, &host->host_uuid) == 0)
                df->host = host;
        }

        if(df->host != host || strncmp(string2str(df->path), parent, parent_len) != 0)
            continue;

        if(!rrd_function_available(host, string2str(df->function)))
            df->status = DYNCFG_STATUS_ORPHAN;

        items[used++] = dictionary_acquired_item_dup(dyncfg_globals.nodes, df_dfe.item);
    }
    dfe_done(df);

    qsort(items, used, sizeof(const DICTIONARY_ITEM *), dyncfg_tree_compar);

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    STRING *last_path = NULL;
    for(size_t i = 0; i < used ;i++) {
        df = dictionary_acquired_item_value(items[i]);
        if(df->path != last_path) {
            last_path = df->path;

            if(i)
                buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, string2str(last_path));
        }

        dyncfg_to_json(df, dictionary_acquired_item_name(items[i]), wb);
    }

    if(used)
        buffer_json_object_close(wb);

    buffer_json_finalize(wb);

    for(size_t i = 0; i < used ;i++)
        dictionary_acquired_item_release(dyncfg_globals.nodes, items[i]);
}

static int dyncfg_config_execute_cb(uuid_t *transaction __maybe_unused, BUFFER *result_body_wb, BUFFER *payload __maybe_unused,
                                    usec_t *stop_monotonic_ut __maybe_unused, const char *function,
                                    void *execute_cb_data,
                                    rrd_function_result_callback_t result_cb, void *result_cb_data,
                                    rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                                    rrd_function_is_cancelled_cb_t is_cancelled_cb __maybe_unused,
                                    void *is_cancelled_cb_data __maybe_unused,
                                    rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                                    void *register_canceller_cb_data __maybe_unused,
                                    rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                                    void *register_progresser_cb_data __maybe_unused) {
    RRDHOST *host = execute_cb_data;
    int code;

    if(strncmp(function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) != 0) {
        code = HTTP_RESP_INTERNAL_SERVER_ERROR;
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: received function that is not config: %s", function);
        rrd_call_function_error(result_body_wb, "wrong function call", code);
    }
    else {
        // extract the id
        const char *id = &function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
        while (*id && isspace(*id))
            id++;
        const char *space = id;
        while (*space && !isspace(*space))
            space++;
        size_t id_len = space - id;

        char id_copy[id_len + 1];
        memcpy(id_copy, id, id_len);
        id_copy[id_len] = '\0';

        // extract the path
        const char *path = space;
        while (*path && isspace(*path))
            path++;
        space = path;
        while (*space && !isspace(*space))
            space++;
        size_t path_len = space - path;

        if (*path || !path_len) {
            path_len = 1;
            path = "/";
        }

        char path_copy[path_len + 1];
        memcpy(path_copy, path, path_len);
        path_copy[path_len] = '\0';

        if (strcmp(id_copy, "tree") == 0) {
            code = HTTP_RESP_OK;
            dyncfg_tree_for_host(host, result_body_wb, path_copy);
        }
        else {
            code = HTTP_RESP_NOT_FOUND;
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: unknown config id '%s' in call: %s", id_copy, function);
            rrd_call_function_error(result_body_wb, "unknown config id given", code);
        }
    }

    if(result_cb)
        result_cb(result_body_wb, code, result_cb_data);

    return code;
}

// ----------------------------------------------------------------------------
// this adds a 'config' function to all leaf nodes (localhost and virtual nodes)
// which is used to serve the tree and act as a catch-all for all config calls
// for which there is no id overloaded.

void dyncfg_host_init(RRDHOST *host) {
    rrd_function_add(host, NULL, PLUGINSD_FUNCTION_CONFIG, 120,
                     1000, "Dynamic configuration", "config",
                     HTTP_ACCESS_MEMBER,
                     true, dyncfg_config_execute_cb, host);
}
