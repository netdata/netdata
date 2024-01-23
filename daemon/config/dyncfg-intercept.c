// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

// ----------------------------------------------------------------------------
// we intercept the config function calls of the plugin

struct dyncfg_call {
    BUFFER *payload;
    char *function;
    char *id;
    char *add_name;
    char *source;
    DYNCFG_CMDS cmd;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
    bool from_dyncfg_echo;
};

DYNCFG_STATUS dyncfg_status_from_successful_response(int code) {
    DYNCFG_STATUS status = DYNCFG_STATUS_ACCEPTED;

    switch(code) {
        default:
        case DYNCFG_RESP_ACCEPTED:
        case DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED:
            status = DYNCFG_STATUS_ACCEPTED;
            break;

        case DYNCFG_RESP_ACCEPTED_DISABLED:
            status = DYNCFG_STATUS_DISABLED;
            break;

        case DYNCFG_RESP_RUNNING:
            status = DYNCFG_STATUS_RUNNING;
            break;

    }

    return status;
}

static void dyncfg_function_intercept_keep_source(DYNCFG *df, const char *source) {
    STRING *old = df->source;
    df->source = string_strdupz(source);
    string_freez(old);
}

void dyncfg_function_intercept_result_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_call *dc = result_cb_data;

    bool called_from_dyncfg_echo = dc->from_dyncfg_echo;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, dc->id, -1);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);
        bool old_user_disabled = df->user_disabled;
        bool save_required = false;

        if (!called_from_dyncfg_echo) {
            // the command was sent by a user

            if (DYNCFG_RESP_SUCCESS(code)) {
                if (dc->cmd == DYNCFG_CMD_ADD) {
                    char id[strlen(dc->id) + 1 + strlen(dc->add_name) + 1];
                    snprintfz(id, sizeof(id), "%s:%s", dc->id, dc->add_name);

                    RRDHOST *host = dyncfg_rrdhost(df);
                    if(!host) {
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "DYNCFG: cannot add job '%s' because host is missing", id);
                    }
                    else {
                        const DICTIONARY_ITEM *new_item = dyncfg_add_internal(
                            host,
                            id,
                            string2str(df->path),
                            dyncfg_status_from_successful_response(code),
                            DYNCFG_TYPE_JOB,
                            DYNCFG_SOURCE_TYPE_DYNCFG,
                            dc->source,
                            (df->cmds & ~DYNCFG_CMD_ADD) | DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST |
                                DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_REMOVE,
                            0,
                            0,
                            df->sync,
                            df->execute_cb,
                            df->execute_cb_data,
                            false);

                        DYNCFG *new_df = dictionary_acquired_item_value(new_item);
                        SWAP(new_df->payload, dc->payload);
                        if (code == DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED)
                            new_df->restart_required = true;

                        dyncfg_file_save(id, new_df);
                        dictionary_acquired_item_release(dyncfg_globals.nodes, new_item);
                    }
                } else if (dc->cmd == DYNCFG_CMD_UPDATE) {
                    df->source_type = DYNCFG_SOURCE_TYPE_DYNCFG;
                    dyncfg_function_intercept_keep_source(df, dc->source);

                    df->status = dyncfg_status_from_successful_response(code);
                    SWAP(df->payload, dc->payload);

                    save_required = true;
                } else if (dc->cmd == DYNCFG_CMD_ENABLE) {
                    df->user_disabled = false;
                    dyncfg_function_intercept_keep_source(df, dc->source);
                } else if (dc->cmd == DYNCFG_CMD_DISABLE) {
                    df->user_disabled = true;
                    dyncfg_function_intercept_keep_source(df, dc->source);
                } else if (dc->cmd == DYNCFG_CMD_REMOVE) {
                    dyncfg_file_delete(dc->id);
                    dictionary_del(dyncfg_globals.nodes, dc->id);
                }

                if(dc->cmd != DYNCFG_CMD_ADD && code == DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED)
                    df->restart_required = true;
            }
            else
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "DYNCFG: plugin returned code %d to user initiated call: %s", code, dc->function);
        }
        else {
            // the command was sent by dyncfg

            if(DYNCFG_RESP_SUCCESS(code)) {
                if(dc->cmd == DYNCFG_CMD_ADD) {
                    char id[strlen(dc->id) + 1 + strlen(dc->add_name) + 1];
                    snprintfz(id, sizeof(id), "%s:%s", dc->id, dc->add_name);

                    const DICTIONARY_ITEM *new_item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, id);
                    if(new_item) {
                        DYNCFG *new_df = dictionary_acquired_item_value(new_item);
                        new_df->status = dyncfg_status_from_successful_response(code);

                        if(code == DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED)
                            new_df->restart_required = true;

                        dictionary_acquired_item_release(dyncfg_globals.nodes, new_item);
                    }
                }
                else if(dc->cmd  == DYNCFG_CMD_UPDATE) {
                    df->status = dyncfg_status_from_successful_response(code);
                    df->plugin_rejected = false;
                }
                else if(dc->cmd == DYNCFG_CMD_DISABLE)
                    df->status = DYNCFG_STATUS_DISABLED;
                else if(dc->cmd == DYNCFG_CMD_ENABLE)
                    df->status = dyncfg_status_from_successful_response(code);

                if(dc->cmd != DYNCFG_CMD_ADD && code == DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED)
                    df->restart_required = true;
            }
            else {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "DYNCFG: plugin returned code %d to dyncfg initiated call: %s", code, dc->function);

                if(dc->cmd & (DYNCFG_CMD_UPDATE | DYNCFG_CMD_ADD))
                    df->plugin_rejected = true;
            }
        }

        if (save_required || old_user_disabled != df->user_disabled)
            dyncfg_file_save(dc->id, df);

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }

    if(dc->result_cb)
        dc->result_cb(wb, code, dc->result_cb_data);

    buffer_free(dc->payload);
    freez(dc->function);
    freez(dc->id);
    freez(dc->source);
    freez(dc->add_name);
    freez(dc);
}

// ----------------------------------------------------------------------------

static void dyncfg_apply_action_on_all_template_jobs(struct rrd_function_execute *rfe, const char *template_id, DYNCFG_CMDS c) {
    STRING *template = string_strdupz(template_id);
    DYNCFG *df;

    size_t all = 0, done = 0;
    dfe_start_read(dyncfg_globals.nodes, df) {
        if(df->template == template && df->type == DYNCFG_TYPE_JOB)
            all++;
    }
    dfe_done(df);

    if(rfe->progress.cb)
        rfe->progress.cb(rfe->progress.data, done, all);

    dfe_start_reentrant(dyncfg_globals.nodes, df) {
        if(df->template == template && df->type == DYNCFG_TYPE_JOB) {
            DYNCFG_CMDS cmd_to_send_to_plugin = c;

            if(c == DYNCFG_CMD_ENABLE)
                cmd_to_send_to_plugin = df->user_disabled ? DYNCFG_CMD_DISABLE : DYNCFG_CMD_ENABLE;
            else if(c == DYNCFG_CMD_DISABLE)
                cmd_to_send_to_plugin = DYNCFG_CMD_DISABLE;

            dyncfg_echo(df_dfe.item, df, df_dfe.name, cmd_to_send_to_plugin);

            if(rfe->progress.cb)
                rfe->progress.cb(rfe->progress.data, ++done, all);
        }
    }
    dfe_done(df);

    string_freez(template);
}

// ----------------------------------------------------------------------------
// the callback for all config functions

int dyncfg_function_intercept_cb(struct rrd_function_execute *rfe, void *data __maybe_unused) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    bool called_from_dyncfg_echo = rrd_function_has_this_original_result_callback(rfe->transaction, dyncfg_echo_cb);

    DYNCFG_CMDS c = DYNCFG_CMD_NONE;
    const DICTIONARY_ITEM *item = NULL;
    const char *add_name = NULL;
    size_t add_name_len = 0;
    if(strncmp(rfe->function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) == 0) {
        const char *id = &rfe->function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
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

    int rc = HTTP_RESP_INTERNAL_SERVER_ERROR;

    if(!item) {
        rc = HTTP_RESP_NOT_FOUND;
        dyncfg_default_response(rfe->result.wb, rc, "dyncfg functions intercept: id is not found");

        if(rfe->result.cb)
            rfe->result.cb(rfe->result.wb, rc, rfe->result.data);

        return HTTP_RESP_NOT_FOUND;
    }

    DYNCFG *df = dictionary_acquired_item_value(item);
    const char *id = dictionary_acquired_item_name(item);
    bool has_payload = rfe->payload && buffer_strlen(rfe->payload) ? true : false;
    bool make_the_call_to_plugin = true;

    if((c & (DYNCFG_CMD_GET | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_REMOVE | DYNCFG_CMD_RESTART)) && has_payload)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: command has a payload, but it is not going to be used: %s", rfe->function);

    if(c == DYNCFG_CMD_NONE) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: this command is unknown: %s", rfe->function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(rfe->result.wb, rc,
                                "dyncfg functions intercept: unknown command");
        make_the_call_to_plugin = false;
    }
    else if(!(df->cmds & c)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: this command is not supported by the configuration node: %s", rfe->function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(rfe->result.wb, rc,
                                "dyncfg functions intercept: this command is not supported by this configuration node");
        make_the_call_to_plugin = false;
    }
    else if((c & (DYNCFG_CMD_ADD | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST)) && !has_payload) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: command requires a payload, but no payload given: %s", rfe->function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(rfe->result.wb, rc,
                                "dyncfg functions intercept: payload is required");
        make_the_call_to_plugin = false;
    }
    else if(c == DYNCFG_CMD_SCHEMA) {
        bool loaded = false;
        if(df->type == DYNCFG_TYPE_JOB) {
            if(df->template)
                loaded = dyncfg_get_schema(string2str(df->template), rfe->result.wb);
        }
        else
            loaded = dyncfg_get_schema(id, rfe->result.wb);

        if(loaded) {
            rfe->result.wb->content_type = CT_APPLICATION_JSON;
            rfe->result.wb->expires = now_realtime_sec();
            rc = HTTP_RESP_OK;
            make_the_call_to_plugin = false;
        }
    }
    else if(c & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_RESTART) && df->type == DYNCFG_TYPE_TEMPLATE) {
        if(!called_from_dyncfg_echo) {
            bool old_user_disabled = df->user_disabled;
            if (c == DYNCFG_CMD_ENABLE)
                df->user_disabled = false;
            else if (c == DYNCFG_CMD_DISABLE)
                df->user_disabled = true;

            if (df->user_disabled != old_user_disabled)
                dyncfg_file_save(id, df);
        }

        dyncfg_apply_action_on_all_template_jobs(rfe, id, c);

        rc = HTTP_RESP_OK;
        dyncfg_default_response(rfe->result.wb, rc, "applied");
        make_the_call_to_plugin = false;
    }
    else if(c == DYNCFG_CMD_ADD) {
        if (df->type != DYNCFG_TYPE_TEMPLATE) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: add command can only be applied on templates, not %s: %s",
                   dyncfg_id2type(df->type), rfe->function);

            rc = HTTP_RESP_BAD_REQUEST;
            dyncfg_default_response(rfe->result.wb, rc,
                                    "dyncfg functions intercept: add command is only allowed in templates");
            make_the_call_to_plugin = false;
        }
        else if (!add_name || !*add_name || !add_name_len) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: add command does not specify a name: %s", rfe->function);

            rc = HTTP_RESP_BAD_REQUEST;
            dyncfg_default_response(rfe->result.wb, rc,
                                    "dyncfg functions intercept: command add requires a name, which is missing");

            make_the_call_to_plugin = false;
        }
    }
    else if(c == DYNCFG_CMD_ENABLE && df->type == DYNCFG_TYPE_JOB && dyncfg_is_user_disabled(string2str(df->template))) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot enable a job of a disabled template: %s", rfe->function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(rfe->result.wb, rc,
                                "dyncfg functions intercept: this job belongs to disabled template");

        make_the_call_to_plugin = false;
    }

    if(make_the_call_to_plugin) {
        struct dyncfg_call *dc = callocz(1, sizeof(*dc));
        dc->function = strdupz(rfe->function);
        dc->id = strdupz(id);
        dc->source = rfe->source ? strdupz(rfe->source) : NULL;
        dc->add_name = (c == DYNCFG_CMD_ADD) ? strndupz(add_name, add_name_len) : NULL;
        dc->cmd = c;
        dc->result_cb = rfe->result.cb;
        dc->result_cb_data = rfe->result.data;
        dc->payload = buffer_dup(rfe->payload);
        dc->from_dyncfg_echo = called_from_dyncfg_echo;

        rfe->result.cb = dyncfg_function_intercept_result_cb;
        rfe->result.data = dc;

        rc = df->execute_cb(rfe, df->execute_cb_data);
    }
    else if(rfe->result.cb)
        rfe->result.cb(rfe->result.wb, rc, rfe->result.data);

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return rc;
}

