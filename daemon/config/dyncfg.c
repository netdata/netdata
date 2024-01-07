// SPDX-License-Identifier: GPL-3.0-or-later

#define RRD_COLLECTOR_INTERNALS
#define RRD_FUNCTIONS_INTERNALS
#define DYNCFG_INTERNALS

#include "dyncfg.h"

struct dyncfg_globals dyncfg_globals = { 0 };

void dyncfg_cleanup(DYNCFG *v) {
    buffer_free(v->payload);
    v->payload = NULL;

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

    if(v->host != nv->host) {
        SWAP(v->host, nv->host);
        changes++;
    }

    if(v->path != nv->path) {
        SWAP(v->path, nv->path);
        changes++;
    }

    if(v->status != nv->status) {
        SWAP(v->status, nv->status);
        changes++;
    }

    if(v->type != nv->type) {
        SWAP(v->type, nv->type);
        changes++;
    }

    if(v->source_type != nv->source_type) {
        SWAP(v->source_type, nv->source_type);
        changes++;
    }

    if(v->cmds != nv->cmds) {
        SWAP(v->cmds, nv->cmds);
        changes++;
    }

    if(v->source != nv->source) {
        SWAP(v->source, nv->source);
        changes++;
    }

    if(nv->created_ut < v->created_ut) {
        SWAP(v->created_ut, nv->created_ut);
        changes++;
    }

    if(nv->modified_ut > v->modified_ut) {
        SWAP(v->modified_ut, nv->modified_ut);
        changes++;
    }

    if(v->sync != nv->sync) {
        SWAP(v->sync, nv->sync);
        changes++;
    }

    if(nv->payload) {
        SWAP(v->payload, nv->payload);
        changes++;
    }

    if(nv->execute_cb && (v->execute_cb != nv->execute_cb || v->execute_cb_data != nv->execute_cb_data)) {
        v->execute_cb = nv->execute_cb;
        v->execute_cb_data = nv->execute_cb_data;
        changes++;
    }

    dyncfg_cleanup(nv);

    return changes > 0;
}

void dyncfg_init_low_level(bool load_saved) {
    if(!dyncfg_globals.nodes) {
        dyncfg_globals.nodes = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(DYNCFG));
        dictionary_register_insert_callback(dyncfg_globals.nodes, dyncfg_insert_cb, NULL);
        dictionary_register_conflict_callback(dyncfg_globals.nodes, dyncfg_conflict_cb, NULL);
        dictionary_register_delete_callback(dyncfg_globals.nodes, dyncfg_delete_cb, NULL);

        char path[PATH_MAX];
        snprintfz(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, "config");

        if(mkdir(path, 0755) == -1) {
            if(errno != EEXIST)
                nd_log(NDLS_DAEMON, NDLP_CRIT, "DYNCFG: failed to create dynamic configuration directory '%s'", path);
        }

        dyncfg_globals.dir = strdupz(path);

        if(load_saved)
            dyncfg_load_all();
    }
}

static const DICTIONARY_ITEM *dyncfg_add_internal(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {
    DYNCFG tmp = {
        .host = host,
        .path = string_strdupz(path),
        .status = status,
        .type = type,
        .cmds = cmds,
        .source_type = source_type,
        .source = string_strdupz(source),
        .created_ut = created_ut,
        .modified_ut = modified_ut,
        .sync = sync,
        .user_disabled = false,
        .restart_required = false,
        .payload = NULL,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
    };
    uuid_copy(tmp.host_uuid, host->host_uuid);

    return dictionary_set_and_acquire_item_advanced(dyncfg_globals.nodes, id, -1, &tmp, sizeof(tmp), NULL);
}

// ----------------------------------------------------------------------------
// echo is the first config command we send to the plugin

struct dyncfg_echo {
    const DICTIONARY_ITEM *item;
    DYNCFG *df;
    BUFFER *wb;
};

void dyncfg_echo_cb(BUFFER *wb __maybe_unused, int code, void *result_cb_data) {
    struct dyncfg_echo *e = result_cb_data;

    if(code != HTTP_RESP_OK) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to send the first config cmd to '%s', with error code %d",
               e->item ? dictionary_acquired_item_name(e->item) : "(null)", code);

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

static void dyncfg_send_echo_status(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id) {
    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;

    char buf[strlen(id) + 100];
    snprintfz(buf, sizeof(buf), PLUGINSD_FUNCTION_CONFIG " %s %s", id, df->user_disabled ? "disable" : "enable");

    rrd_function_run(df->host, e->wb, 10, HTTP_ACCESS_ADMIN, buf, false, NULL,
                     dyncfg_echo_cb, e,
                     NULL, NULL,
                     NULL, NULL,
                     NULL);
}

static void dyncfg_send_echo_payload(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, const char *cmd) {
    if(!df->payload)
        return;

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;

    char buf[strlen(id) + 100];
    snprintfz(buf, sizeof(buf), PLUGINSD_FUNCTION_CONFIG " %s %s", id, cmd);

    rrd_function_run(df->host, e->wb, 10, HTTP_ACCESS_ADMIN, buf, false, NULL,
                     dyncfg_echo_cb, e,
                     NULL, NULL,
                     NULL, NULL,
                     NULL);
}

static void dyncfg_send_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id) {
    dyncfg_send_echo_payload(item, df, id, "update");
}

static void dyncfg_send_echo_add(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, const char *job_name) {
    char buf[strlen(job_name) + 20];
    snprintfz(buf, sizeof(buf), "add %s", job_name);
    dyncfg_send_echo_payload(item, df, id, buf);
}

// ----------------------------------------------------------------------------
// we intercept the config function calls of the plugin

struct dyncfg_call {
    BUFFER *payload;
    char *function;
    char *id;
    char *add_name;
    DYNCFG_CMDS cmd;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
};

void dyncfg_function_result_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_call *dc = result_cb_data;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, dc->id, -1);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);

        if(code == HTTP_RESP_NOT_FOUND) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: plugin returned not found error to call '%s', marking it as rejected.", dc->function);
            df->status = DYNCFG_STATUS_REJECTED;
        }
        else if(code == HTTP_RESP_NOT_IMPLEMENTED) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: plugin returned not supported error to call '%s', disabling this action.", dc->function);
            df->cmds &= ~dc->cmd;
        }
        else if(code == HTTP_RESP_ACCEPTED) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "DYNCFG: plugin returned 202 to call '%s', restart is required.", dc->function);
            df->cmds &= ~dc->cmd;
        }

        if(code == HTTP_RESP_OK || code == HTTP_RESP_ACCEPTED) {
            if(dc->cmd == DYNCFG_CMD_ADD) {
                char id[strlen(dc->id) + 1 + strlen(dc->add_name) + 1];
                snprintfz(id, sizeof(id), "%s:%s", dc->id, dc->add_name);

                const DICTIONARY_ITEM *new_item =
                    dyncfg_add_internal(df->host, id, string2str(df->path),
                                        DYNCFG_STATUS_OK, DYNCFG_TYPE_JOB, DYNCFG_SOURCE_TYPE_DYNCFG,
                                        "dyncfg", df->cmds & ~DYNCFG_CMD_ADD, 0, 0,
                                        df->sync, NULL, NULL);

                DYNCFG *new_df = dictionary_acquired_item_value(new_item);
                SWAP(new_df->payload, dc->payload);

                dyncfg_save(id, new_df);
                dictionary_acquired_item_release(dyncfg_globals.nodes, new_item);
            }
            else if(dc->cmd == DYNCFG_CMD_UPDATE) {
                SWAP(df->payload, dc->payload);
                dyncfg_save(dc->id, df);
            }
            else if(dc->cmd == DYNCFG_CMD_ENABLE) {
                bool old = df->user_disabled;
                df->user_disabled = false;

                if(old != df->user_disabled)
                    dyncfg_save(dc->id, df);
            }
            else if(dc->cmd == DYNCFG_CMD_DISABLE) {
                bool old = df->user_disabled;
                df->user_disabled = true;

                if(old != df->user_disabled)
                    dyncfg_save(dc->id, df);
            }
        }

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }

    if(dc->result_cb)
        dc->result_cb(wb, code, dc->result_cb_data);

    buffer_free(dc->payload);
    freez(dc->function);
    freez(dc->id);
    freez(dc);
}

// ----------------------------------------------------------------------------
// the callback for all config functions

static int dyncfg_function_execute_cb(uuid_t *transaction, BUFFER *result_body_wb, BUFFER *payload,
                                 usec_t *stop_monotonic_ut, const char *function,
                                 void *execute_cb_data __maybe_unused,
                                 rrd_function_result_callback_t result_cb, void *result_cb_data,
                                 rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                 rrd_function_is_cancelled_cb_t is_cancelled_cb,
                                 void *is_cancelled_cb_data,
                                 rrd_function_register_canceller_cb_t register_canceller_cb,
                                 void *register_canceller_cb_data,
                                 rrd_function_register_progresser_cb_t register_progresser_cb,
                                 void *register_progresser_cb_data) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    DYNCFG_CMDS c = DYNCFG_CMD_NONE;
    const DICTIONARY_ITEM *item = NULL;
    const char *add_name = NULL;
    size_t add_name_len = 0;
    if(strncmp(function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) == 0) {
        const char *id = &function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
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

        if(c == DYNCFG_CMD_ADD) {
            add_name = space;
            while(isspace(*add_name)) add_name++;
            space = add_name;
            while(*space && !isspace(*space)) space++;
            add_name_len = space - add_name;
        }

        item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, id, (ssize_t)id_len);
    }

    if(!item) {
        rrd_call_function_error(result_body_wb, "not found", HTTP_RESP_NOT_FOUND);

        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_NOT_FOUND, result_cb_data);

        return HTTP_RESP_NOT_FOUND;
    }

    if(c == DYNCFG_CMD_ADD && (!add_name || !*add_name || !add_name_len)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: add command does not specify a name: %s", function);
        dictionary_acquired_item_release(dyncfg_globals.nodes, item);

        rrd_call_function_error(result_body_wb, "bad request, name is missing", HTTP_RESP_BAD_REQUEST);

        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_BAD_REQUEST, result_cb_data);

        return HTTP_RESP_BAD_REQUEST;
    }

    struct dyncfg_call *dc = callocz(1, sizeof(*dc));
    dc->function = strdupz(function);
    dc->id = strdupz(dictionary_acquired_item_name(item));
    dc->add_name = (c == DYNCFG_CMD_ADD) ? strndupz(add_name, add_name_len) : NULL;
    dc->cmd = c;
    dc->result_cb = result_cb;
    dc->result_cb_data = result_cb_data;
    dc->payload = buffer_dup(payload);

    DYNCFG *df = dictionary_acquired_item_value(item);
    int rc = df->execute_cb(transaction, result_body_wb, payload,
                            stop_monotonic_ut, function, df->execute_cb_data,
                            dyncfg_function_result_cb, dc,
                            progress_cb, progress_cb_data,
                            is_cancelled_cb, is_cancelled_cb_data,
                            register_canceller_cb, register_canceller_cb_data,
                            register_progresser_cb, register_progresser_cb_data);

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return rc;
}

// ----------------------------------------------------------------------------

static void dyncfg_update_plugin(const char *id) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, id, -1);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: asked to update plugin for configuration '%s', but it is not found.", id);
        return;
    }

    DYNCFG *df = dictionary_acquired_item_value(item);

    if(df->type == DYNCFG_TYPE_SINGLE || df->type == DYNCFG_TYPE_JOB) {
        if (df->cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
            dyncfg_send_echo_update(item, df, id);
    }
    else if(df->type == DYNCFG_TYPE_TEMPLATE) {
        size_t len = strlen(id);
        DYNCFG *tf;
        dfe_start_reentrant(dyncfg_globals.nodes, tf) {
            const char *t_id = tf_dfe.name;
            if(tf->type == DYNCFG_TYPE_JOB && strncmp(t_id, id, len) == 0 && t_id[len] == ':' && t_id[len + 1]) {
                if (tf->cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
                    dyncfg_send_echo_add(tf_dfe.item, tf, id, &t_id[len + 1]);
            }
        }
        dfe_done(tf);
    }

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
}

bool dyncfg_add_low_level(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {
    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return false;
    }

    DYNCFG_CMDS old_cmds = cmds;

    // all configurations support schema
    cmds |= DYNCFG_CMD_SCHEMA;

    // if there is either enable or disable, both are supported
    if(cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
        cmds |= DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE;

    // add
    if(type == DYNCFG_TYPE_TEMPLATE) {
        // templates must always support "add"
        cmds |= DYNCFG_CMD_ADD;
    }
    else {
        // only templates can have "add"
        cmds &= ~DYNCFG_CMD_ADD;
    }

    // remove
    if(source_type == DYNCFG_SOURCE_TYPE_DYNCFG && type != DYNCFG_TYPE_JOB) {
        // remove is only available for dyncfg jobs
        cmds |= DYNCFG_CMD_REMOVE;
    }
    else {
        // remove is only available for dyncfg jobs
        cmds &= ~DYNCFG_CMD_REMOVE;
    }

    // data
    if(type == DYNCFG_TYPE_TEMPLATE) {
        // templates do not have data
        cmds &= ~(DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST);
    }

    if(cmds != old_cmds) {
        CLEAN_BUFFER *t = buffer_create(1024, NULL);
        buffer_sprintf(t, "DYNCFG: id '%s' was declared with cmds: ", id);
        dyncfg_cmds2buffer(old_cmds, t);
        buffer_strcat(t, ", but they have sanitized to: ");
        dyncfg_cmds2buffer(cmds, t);
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "%s", buffer_tostring(t));
    }

    const DICTIONARY_ITEM *item = dyncfg_add_internal(host, id, path, status, type, source_type, source, cmds, created_ut, modified_ut, sync, execute_cb, execute_cb_data);
    DYNCFG *df = dictionary_acquired_item_value(item);

    char name[strlen(id) + 20];
    snprintfz(name, sizeof(name), PLUGINSD_FUNCTION_CONFIG " %s", id);

    rrd_collector_started();
    rrd_function_add(host, NULL, name, 120, 1000,
                     "Dynamic configuration", "config",HTTP_ACCESS_MEMBER,
                     sync, dyncfg_function_execute_cb, NULL);

    dyncfg_send_echo_status(item, df, id);
    dyncfg_update_plugin(id);
    dictionary_acquired_item_release(dyncfg_globals.nodes, item);

    return true;
}

void dyncfg_del_low_level(RRDHOST *host, const char *id) {
    dictionary_del(dyncfg_globals.nodes, id);

    char name[strlen(id) + 20];
    snprintfz(name, sizeof(name), PLUGINSD_FUNCTION_CONFIG " %s", id);
    rrd_function_del(host, NULL, name);
}

// ----------------------------------------------------------------------------

void dyncfg_add_streaming(BUFFER *wb) {
    // when sending config functions to parents, we send only 1 function called 'config';
    // the parent will send the command to the child and the child will validate it;
    // this way the parent does not need to receive removals of config functions;

    buffer_sprintf(wb
                   , PLUGINSD_KEYWORD_FUNCTION " GLOBAL " PLUGINSD_FUNCTION_CONFIG " %d \"%s\" \"%s\" \"%s\" %d\n"
                   , 120
                   , "Dynamic configuration"
                   , "config"
                   , http_id2access(HTTP_ACCESS_MEMBER)
                   , 1000
    );
}

bool dyncfg_available_for_rrdhost(RRDHOST *host) {
    if(host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST))
        return true;

    if(!host->functions)
        return false;

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->functions, PLUGINSD_FUNCTION_CONFIG);
    if(item) {
        struct rrd_host_function *rdcf = dictionary_acquired_item_value(item);
        if(rrd_collector_running(rdcf->collector))
            ret = true;

        dictionary_acquired_item_release(host->functions, item);
    }

    return ret;
}

// ----------------------------------------------------------------------------

