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
    DYNCFG_CMDS cmd;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
};

void dyncfg_function_result_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_call *dc = result_cb_data;

    bool called_from_dyncfg_echo = dc->result_cb == dyncfg_echo_cb ? true : false;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, dc->id, -1);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);
        bool old_user_disabled = df->user_disabled;
        bool save_required = false;

        if(code == HTTP_RESP_NOT_FOUND) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: plugin returned not found error to call '%s', marking config node as rejected.", dc->function);
            df->status = DYNCFG_STATUS_REJECTED;
        }
        else if(code == HTTP_RESP_NOT_IMPLEMENTED) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: plugin returned not supported error to call '%s', disabling this action for this config node.", dc->function);
            df->cmds &= ~dc->cmd;
        }
        else if(code == HTTP_RESP_ACCEPTED) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "DYNCFG: plugin returned 202 to call '%s', restart is required.", dc->function);
            df->restart_required = true;
        }

        if (!called_from_dyncfg_echo) {
            if (code == HTTP_RESP_OK || code == HTTP_RESP_ACCEPTED) {
                if (dc->cmd == DYNCFG_CMD_ADD) {
                    char id[strlen(dc->id) + 1 + strlen(dc->add_name) + 1];
                    snprintfz(id, sizeof(id), "%s:%s", dc->id, dc->add_name);

                    const DICTIONARY_ITEM *new_item = dyncfg_add_internal(
                        df->host,
                        id,
                        string2str(df->path),
                        DYNCFG_STATUS_OK,
                        DYNCFG_TYPE_JOB,
                        DYNCFG_SOURCE_TYPE_DYNCFG,
                        "dyncfg",
                        df->cmds & ~DYNCFG_CMD_ADD,
                        0,
                        0,
                        df->sync,
                        NULL,
                        NULL);

                    DYNCFG *new_df = dictionary_acquired_item_value(new_item);
                    SWAP(new_df->payload, dc->payload);

                    dyncfg_file_save(id, new_df);
                    dictionary_acquired_item_release(dyncfg_globals.nodes, new_item);
                } else if (dc->cmd == DYNCFG_CMD_UPDATE) {
                    SWAP(df->payload, dc->payload);
                    save_required = true;
                } else if (dc->cmd == DYNCFG_CMD_ENABLE) {
                    df->user_disabled = false;
                } else if (dc->cmd == DYNCFG_CMD_DISABLE) {
                    df->user_disabled = true;
                } else if (dc->cmd == DYNCFG_CMD_REMOVE) {
                    dyncfg_file_delete(dc->id);
                }
            }

            if (save_required || old_user_disabled != df->user_disabled)
                dyncfg_file_save(dc->id, df);
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

static void dyncfg_apply_action_on_all_template_jobs(const char *template_id, DYNCFG_CMDS c) {
    STRING *template = string_strdupz(template_id);

    DYNCFG *df;
    dfe_start_read(dyncfg_globals.nodes, df) {
        if(df->template == template && df->type == DYNCFG_TYPE_JOB)
            dyncfg_echo(df_dfe.item, df, df_dfe.name, c);
    }
    dfe_done(df);
}

// ----------------------------------------------------------------------------
// the callback for all config functions

int dyncfg_function_execute_cb(uuid_t *transaction, BUFFER *result_body_wb, BUFFER *payload,
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

    bool called_from_dyncfg_echo = result_cb == dyncfg_echo_cb ? true : false;

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

    int rc = HTTP_RESP_INTERNAL_SERVER_ERROR;

    if(!item) {
        rc = HTTP_RESP_NOT_FOUND;
        dyncfg_default_response(result_body_wb, rc, "dyncfg functions intercept: id is not found");

        if(result_cb)
            result_cb(result_body_wb, rc, result_cb_data);

        return HTTP_RESP_NOT_FOUND;
    }

    DYNCFG *df = dictionary_acquired_item_value(item);
    const char *id = dictionary_acquired_item_name(item);
    bool has_payload = payload && buffer_strlen(payload) ? true : false;
    bool make_the_call_to_plugin = true;

    if((c & (DYNCFG_CMD_GET | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_REMOVE | DYNCFG_CMD_RESTART)) && has_payload)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: command has a payload, but it is not going to be used: %s", function);

    if(c == DYNCFG_CMD_NONE) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: this command is unknown: %s", function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(result_body_wb, rc,
                                "dyncfg functions intercept: unknown command");
        make_the_call_to_plugin = false;
    }
    else if(!(df->cmds & c)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: this command is not supported by the configuration node: %s", function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(result_body_wb, rc,
                                "dyncfg functions intercept: this command is not supported by this configuration node");
        make_the_call_to_plugin = false;
    }
    else if((c & (DYNCFG_CMD_ADD | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST)) && !has_payload) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: command requires a payload, but no payload given: %s", function);

        rc = HTTP_RESP_BAD_REQUEST;
        dyncfg_default_response(result_body_wb, rc,
                                "dyncfg functions intercept: payload is required");
        make_the_call_to_plugin = false;
    }
    else if(c == DYNCFG_CMD_SCHEMA) {
        bool loaded = false;
        if(df->type == DYNCFG_TYPE_JOB) {
            char template[strlen(id) + 1];
            memcpy(template, id, sizeof(template));
            char *colon = strrchr(template, ':');
            if(colon) *colon = '\0';
            if(template[0])
                loaded = dyncfg_get_schema(template, result_body_wb);
        }
        else
            loaded = dyncfg_get_schema(id, result_body_wb);

        if(loaded) {
            result_body_wb->content_type = CT_APPLICATION_JSON;
            result_body_wb->expires = now_realtime_sec();
            rc = HTTP_RESP_OK;
            make_the_call_to_plugin = false;
        }
    }
    else if(c & (DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE|DYNCFG_CMD_RESTART) && df->type == DYNCFG_TYPE_TEMPLATE) {
        if(!called_from_dyncfg_echo) {
            bool old_user_disabled = df->user_disabled;
            if (c == DYNCFG_CMD_ENABLE)
                df->user_disabled = false;
            else if (c == DYNCFG_CMD_DISABLE)
                df->user_disabled = true;

            if (df->user_disabled != old_user_disabled)
                dyncfg_file_save(id, df);
        }

        dyncfg_apply_action_on_all_template_jobs(id, c);

        rc = HTTP_RESP_OK;
        dyncfg_default_response(result_body_wb, rc, "applied");
        make_the_call_to_plugin = false;
    }
    else  if(c == DYNCFG_CMD_ADD) {
        if (df->type != DYNCFG_TYPE_TEMPLATE) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: add command can only be applied on templates, not %s: %s",
                   dyncfg_id2type(df->type), function);

            rc = HTTP_RESP_BAD_REQUEST;
            dyncfg_default_response(result_body_wb, rc,
                                    "dyncfg functions intercept: add command is only allowed in templates");
            make_the_call_to_plugin = false;
        }
        else if (!add_name || !*add_name || !add_name_len) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: add command does not specify a name: %s", function);

            rc = HTTP_RESP_BAD_REQUEST;
            dyncfg_default_response(result_body_wb, rc,
                                    "dyncfg functions intercept: command add requires a name, which is missing");

            make_the_call_to_plugin = false;
        }
    }

    if(make_the_call_to_plugin) {
        struct dyncfg_call *dc = callocz(1, sizeof(*dc));
        dc->function = strdupz(function);
        dc->id = strdupz(id);
        dc->add_name = (c == DYNCFG_CMD_ADD) ? strndupz(add_name, add_name_len) : NULL;
        dc->cmd = c;
        dc->result_cb = result_cb;
        dc->result_cb_data = result_cb_data;
        dc->payload = buffer_dup(payload);

        rc = df->execute_cb(
            transaction,
            result_body_wb,
            payload,
            stop_monotonic_ut,
            function,
            df->execute_cb_data,
            dyncfg_function_result_cb,
            dc,
            progress_cb,
            progress_cb_data,
            is_cancelled_cb,
            is_cancelled_cb_data,
            register_canceller_cb,
            register_canceller_cb_data,
            register_progresser_cb,
            register_progresser_cb_data);
    }
    else if(result_cb)
        result_cb(result_body_wb, rc, result_cb_data);

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return rc;
}

