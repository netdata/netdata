// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

struct dyncfg_call {
    ND_UUID transaction;
    BUFFER *payload;
    const char *function;
    const char *id;
    const char *add_name;
    const char *source;
    DYNCFG_CMDS cmd;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
    bool from_dyncfg_echo;
};

// ----------------------------------------------------------------------------

ENUM_STR_MAP_DEFINE(DYNCFG_CMDS) = {
    { DYNCFG_CMD_GET, "get" },
    { DYNCFG_CMD_SCHEMA, "schema" },
    { DYNCFG_CMD_UPDATE, "update" },
    { DYNCFG_CMD_ADD, "add" },
    { DYNCFG_CMD_TEST, "test" },
    { DYNCFG_CMD_REMOVE, "remove" },
    { DYNCFG_CMD_ENABLE, "enable" },
    { DYNCFG_CMD_DISABLE, "disable" },
    { DYNCFG_CMD_RESTART, "restart" },
    { DYNCFG_CMD_USERCONFIG, "userconfig" },

    // terminator
    { 0, NULL }
};

ENUM_STR_DEFINE_FUNCTIONS(DYNCFG_CMDS, DYNCFG_CMD_NONE, "none");

static void dyncfg_log_user_action(DYNCFG *df, struct dyncfg_call *dc) {
    if(dc->cmd == DYNCFG_CMD_USERCONFIG || dc->cmd == DYNCFG_CMD_GET || dc->cmd == DYNCFG_CMD_SCHEMA)
        return;

    const char *type;
    switch(df->type) {
        default:
        case DYNCFG_TYPE_SINGLE:
            type = "on";
            break;

        case DYNCFG_TYPE_TEMPLATE:
            type = "on template";
            break;
        case DYNCFG_TYPE_JOB:
            type = "on job";
            break;
    }

    PARSED_REQUEST_SOURCE req;
    if(!parse_request_source(dc->source, &req)) {
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_MODULE, "DYNCFG"),
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, localhost->hostname),
            ND_LOG_FIELD_TXT(NDF_REQUEST, dc->function),
            ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &dc->transaction.uuid),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &dyncfg_user_action_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "DYNCFG USER ACTION '%s' %s%s%s '%s' from source: %s",
               DYNCFG_CMDS_2str(dc->cmd),
               dc->add_name ? dc->add_name : "",
               dc->add_name ? " " : "",
               type, dc->id, dc->source);

        return;
    }

    char access_str[1024];
    http_access2txt(access_str, sizeof(access_str), " ", req.access);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_MODULE, "DYNCFG"),
        ND_LOG_FIELD_STR(NDF_NIDL_NODE, localhost->hostname),
        ND_LOG_FIELD_TXT(NDF_REQUEST, dc->function),
        ND_LOG_FIELD_UUID(NDF_TRANSACTION_ID, &dc->transaction.uuid),
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &dyncfg_user_action_msgid),

        ND_LOG_FIELD_UUID(NDF_ACCOUNT_ID, &req.cloud_account_id.uuid),
        ND_LOG_FIELD_TXT(NDF_SRC_IP, req.client_ip),
        ND_LOG_FIELD_TXT(NDF_SRC_FORWARDED_FOR, req.forwarded_for),
        ND_LOG_FIELD_TXT(NDF_USER_NAME, req.client_name),
        ND_LOG_FIELD_TXT(NDF_USER_ROLE, http_id2user_role(req.user_role)),
        ND_LOG_FIELD_CB(NDF_USER_ACCESS, log_cb_http_access_to_hex, &req.access),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "DYNCFG USER ACTION '%s' %s%s%s '%s' by user '%s', IP '%s'",
           DYNCFG_CMDS_2str(dc->cmd),
           dc->add_name ? dc->add_name : "",
           dc->add_name ? " " : "",
           type, dc->id, req.client_name,
           req.forwarded_for[0] ? req.forwarded_for : req.client_ip);
}

// ----------------------------------------------------------------------------
// we intercept the config function calls of the plugin

static void dyncfg_function_intercept_job_successfully_added(DYNCFG *df_template, int code, struct dyncfg_call *dc) {
    char id[strlen(dc->id) + 1 + strlen(dc->add_name) + 1];
    snprintfz(id, sizeof(id), "%s:%s", dc->id, dc->add_name);

    RRDHOST *host = dyncfg_rrdhost(df_template);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "DYNCFG: cannot add job '%s' because host is missing", id);
    }
    else {
        const DICTIONARY_ITEM *item = dyncfg_add_internal(
            host,
            id,
            string2str(df_template->path),
            dyncfg_status_from_successful_response(code),
            DYNCFG_TYPE_JOB,
            DYNCFG_SOURCE_TYPE_DYNCFG,
            dc->source,
            (df_template->cmds & ~DYNCFG_CMD_ADD) | DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST |
                DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_REMOVE,
            0,
            0,
            df_template->sync,
            df_template->view_access,
            df_template->edit_access,
            df_template->execute_cb,
            df_template->execute_cb_data,
            false);

        // adding does not create df->dyncfg
        // we have to do it here

        DYNCFG *df = dictionary_acquired_item_value(item);
        SWAP(df->dyncfg.payload, dc->payload);
        dyncfg_set_dyncfg_source_from_txt(df, dc->source);
        df->dyncfg.user_disabled = false;
        df->dyncfg.source_type = DYNCFG_SOURCE_TYPE_DYNCFG;
        df->dyncfg.status = dyncfg_status_from_successful_response(code);

        dyncfg_file_save(id, df); // updates also the df->dyncfg timestamps
        dyncfg_update_status_on_successful_add_or_update(df, code);

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }
}

static void dyncfg_function_intercept_job_successfully_updated(DYNCFG *df, int code, struct dyncfg_call *dc) {
    df->dyncfg.status = dyncfg_status_from_successful_response(code);
    df->dyncfg.source_type = DYNCFG_SOURCE_TYPE_DYNCFG;
    SWAP(df->dyncfg.payload, dc->payload);
    dyncfg_set_dyncfg_source_from_txt(df, dc->source);

    dyncfg_update_status_on_successful_add_or_update(df, code);
}

void dyncfg_function_intercept_result_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_call *dc = result_cb_data;

    bool called_from_dyncfg_echo = dc->from_dyncfg_echo;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, dc->id, -1);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);
        bool old_user_disabled = df->dyncfg.user_disabled;
        bool save_required = false;

        if (!called_from_dyncfg_echo) {
            // the command was sent by a user

            if (DYNCFG_RESP_SUCCESS(code)) {
                if (dc->cmd == DYNCFG_CMD_ADD) {
                    dyncfg_function_intercept_job_successfully_added(df, code, dc);
                } else if (dc->cmd == DYNCFG_CMD_UPDATE) {
                    dyncfg_function_intercept_job_successfully_updated(df, code, dc);
                    save_required = true;
                }
                else if (dc->cmd == DYNCFG_CMD_ENABLE) {
                    df->dyncfg.user_disabled = false;
                }
                else if (dc->cmd == DYNCFG_CMD_DISABLE) {
                    df->dyncfg.user_disabled = true;
                }
                else if (dc->cmd == DYNCFG_CMD_REMOVE) {
                    dyncfg_file_delete(dc->id);
                    dictionary_del(dyncfg_globals.nodes, dc->id);
                }

                if (save_required || old_user_disabled != df->dyncfg.user_disabled)
                    dyncfg_file_save(dc->id, df);

                dyncfg_log_user_action(df, dc);
            }
            else
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "DYNCFG: plugin returned code %d to user initiated call: %s", code, dc->function);
        }
        else {
            // the command was sent by dyncfg
            // these are handled by the echo callback, we don't need to do anything here
            ;
        }

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }

    if(dc->result_cb)
        dc->result_cb(wb, code, dc->result_cb_data);

    buffer_free(dc->payload);
    freez((void *)dc->function);
    freez((void *)dc->id);
    freez((void *)dc->source);
    freez((void *)dc->add_name);
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
                cmd_to_send_to_plugin = df->dyncfg.user_disabled ? DYNCFG_CMD_DISABLE : DYNCFG_CMD_ENABLE;
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

static int dyncfg_intercept_early_error(struct rrd_function_execute *rfe, int rc, const char *msg) {
    rc = dyncfg_default_response(rfe->result.wb, rc, msg);

    if(rfe->result.cb)
        rfe->result.cb(rfe->result.wb, rc, rfe->result.data);

    return rc;
}

const DICTIONARY_ITEM *dyncfg_get_template_of_new_job(const char *job_id) {
    char id_copy[strlen(job_id) + 1];
    memcpy(id_copy, job_id, sizeof(id_copy));

    char *colon = strrchr(id_copy, ':');
    if(!colon) return NULL;

    *colon = '\0';
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, id_copy);
    if(!item) return NULL;

    DYNCFG *df = dictionary_acquired_item_value(item);
    if(df->type != DYNCFG_TYPE_TEMPLATE) {
        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
        return NULL;
    }

    return item;
}

int dyncfg_function_intercept_cb(struct rrd_function_execute *rfe, void *data __maybe_unused) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    bool called_from_dyncfg_echo = rrd_function_has_this_original_result_callback(rfe->transaction, dyncfg_echo_cb);
    bool has_payload = rfe->payload && buffer_strlen(rfe->payload) ? true : false;
    bool make_the_call_to_plugin = true;

    int rc = HTTP_RESP_INTERNAL_SERVER_ERROR;
    DYNCFG_CMDS cmd;
    const DICTIONARY_ITEM *item = NULL;

    char buf[strlen(rfe->function) + 1];
    memcpy(buf, rfe->function, sizeof(buf));

    char *words[20];
    size_t num_words = quoted_strings_splitter_whitespace(buf, words, 20);

    size_t i = 0;
    char *config = get_word(words, num_words, i++);
    char *id = get_word(words, num_words, i++);
    char *cmd_str = get_word(words, num_words, i++);
    char *add_name = get_word(words, num_words, i++);

    if(!config || !*config || strcmp(config, PLUGINSD_FUNCTION_CONFIG) != 0)
        return dyncfg_intercept_early_error(
            rfe, HTTP_RESP_BAD_REQUEST,
            "dyncfg functions intercept: this is not a dyncfg request");

    cmd = dyncfg_cmds2id(cmd_str);
    if(cmd == DYNCFG_CMD_NONE)
        return dyncfg_intercept_early_error(
            rfe, HTTP_RESP_BAD_REQUEST,
            "dyncfg functions intercept: invalid command received");

    if(cmd == DYNCFG_CMD_ADD || cmd == DYNCFG_CMD_TEST || cmd == DYNCFG_CMD_USERCONFIG) {
        if(cmd == DYNCFG_CMD_TEST && (!add_name || !*add_name)) {
            // backwards compatibility for TEST without a name
            char *colon = strrchr(id, ':');
            if(colon) {
                *colon = '\0';
                add_name = ++colon;
            }
            else
                add_name = "test";
        }

        if(!add_name || !*add_name)
            return dyncfg_intercept_early_error(
                rfe, HTTP_RESP_BAD_REQUEST,
                "dyncfg functions intercept: this action requires a name");

        if(!called_from_dyncfg_echo) {
            char nid[strlen(id) + strlen(add_name) + 2];
            snprintfz(nid, sizeof(nid), "%s:%s", id, add_name);

            if (cmd == DYNCFG_CMD_ADD && dictionary_get(dyncfg_globals.nodes, nid))
                return dyncfg_intercept_early_error(
                    rfe, HTTP_RESP_BAD_REQUEST,
                    "dyncfg functions intercept: a configuration with this name already exists");
        }
    }

    if((cmd == DYNCFG_CMD_ADD || cmd == DYNCFG_CMD_UPDATE || cmd == DYNCFG_CMD_TEST || cmd == DYNCFG_CMD_USERCONFIG) && !has_payload)
        return dyncfg_intercept_early_error(
            rfe, HTTP_RESP_BAD_REQUEST,
            "dyncfg functions intercept: this action requires a payload");

    if((cmd != DYNCFG_CMD_ADD && cmd != DYNCFG_CMD_UPDATE && cmd != DYNCFG_CMD_TEST && cmd != DYNCFG_CMD_USERCONFIG) && has_payload)
        return dyncfg_intercept_early_error(
            rfe, HTTP_RESP_BAD_REQUEST,
            "dyncfg functions intercept: this action does not require a payload");

    item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, id);
    if(!item) {
        if(cmd == DYNCFG_CMD_TEST || cmd == DYNCFG_CMD_USERCONFIG) {
            // this may be a test on a new job
            item = dyncfg_get_template_of_new_job(id);
        }

        if(!item)
            return dyncfg_intercept_early_error(
                rfe, HTTP_RESP_NOT_FOUND,
                "dyncfg functions intercept: id is not found");
    }

    DYNCFG *df = dictionary_acquired_item_value(item);

    // 1. check the permissions of the request

    switch(cmd) {
        case DYNCFG_CMD_GET:
        case DYNCFG_CMD_SCHEMA:
        case DYNCFG_CMD_USERCONFIG:
            if(!http_access_user_has_enough_access_level_for_endpoint(rfe->user_access, df->view_access)) {
                make_the_call_to_plugin = false;
                rc = dyncfg_default_response(
                    rfe->result.wb, HTTP_RESP_FORBIDDEN,
                    "dyncfg: you don't have enough view permissions to execute this command");
            }
            break;

        case DYNCFG_CMD_ENABLE:
        case DYNCFG_CMD_DISABLE:
        case DYNCFG_CMD_ADD:
        case DYNCFG_CMD_TEST:
        case DYNCFG_CMD_UPDATE:
        case DYNCFG_CMD_REMOVE:
        case DYNCFG_CMD_RESTART:
            if(!http_access_user_has_enough_access_level_for_endpoint(rfe->user_access, df->edit_access)) {
                make_the_call_to_plugin = false;
                rc = dyncfg_default_response(
                    rfe->result.wb, HTTP_RESP_FORBIDDEN,
                    "dyncfg: you don't have enough edit permissions to execute this command");
            }
            break;

        default: {
            make_the_call_to_plugin = false;
            rc = dyncfg_default_response(
                rfe->result.wb, HTTP_RESP_INTERNAL_SERVER_ERROR,
                "dyncfg: permissions for this command are not set");
        }
        break;
    }

    // 2. validate the request parameters

    if(make_the_call_to_plugin) {
        if (!(df->cmds & cmd)) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "DYNCFG: this command is not supported by the configuration node: %s", rfe->function);

            make_the_call_to_plugin = false;
            rc = dyncfg_default_response(
                rfe->result.wb, HTTP_RESP_BAD_REQUEST,
                "dyncfg functions intercept: this command is not supported by this configuration node");
        }
        else if (cmd == DYNCFG_CMD_ADD) {
            if (df->type != DYNCFG_TYPE_TEMPLATE) {
                make_the_call_to_plugin = false;
                rc = dyncfg_default_response(
                    rfe->result.wb, HTTP_RESP_BAD_REQUEST,
                    "dyncfg functions intercept: add command is only allowed in templates");

                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "DYNCFG: add command can only be applied on templates, not %s: %s",
                       dyncfg_id2type(df->type), rfe->function);
            }
        }
        else if (
            cmd == DYNCFG_CMD_ENABLE && df->type == DYNCFG_TYPE_JOB &&
            dyncfg_is_user_disabled(string2str(df->template))) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "DYNCFG: cannot enable a job of a disabled template: %s",
                   rfe->function);

            make_the_call_to_plugin = false;
            rc = dyncfg_default_response(
                rfe->result.wb, HTTP_RESP_BAD_REQUEST,
                "dyncfg functions intercept: this job belongs to disabled template");
        }
    }

    // 3. check if it is one of the commands we should execute

    if(make_the_call_to_plugin) {
        if (cmd & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_RESTART) && df->type == DYNCFG_TYPE_TEMPLATE) {
            if (!called_from_dyncfg_echo) {
                bool old_user_disabled = df->dyncfg.user_disabled;
                if (cmd == DYNCFG_CMD_ENABLE)
                    df->dyncfg.user_disabled = false;
                else if (cmd == DYNCFG_CMD_DISABLE)
                    df->dyncfg.user_disabled = true;

                if (df->dyncfg.user_disabled != old_user_disabled)
                    dyncfg_file_save(id, df);

                // log it
                {
                    struct dyncfg_call dc = {
                        .function = rfe->function,
                        .id = id,
                        .source = rfe->source,
                        .add_name = add_name,
                        .cmd = cmd,
                        .result_cb = NULL,
                        .result_cb_data = NULL,
                        .payload = rfe->payload,
                        .from_dyncfg_echo = called_from_dyncfg_echo,
                    };
                    uuid_copy(dc.transaction.uuid, *rfe->transaction);

                    dyncfg_log_user_action(df, &dc);
                }
            }

            dyncfg_apply_action_on_all_template_jobs(rfe, id, cmd);

            rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_OK, "applied to all template job");
            make_the_call_to_plugin = false;
        }
        else if (cmd == DYNCFG_CMD_SCHEMA) {
            bool loaded = false;
            if (df->type == DYNCFG_TYPE_JOB) {
                if (df->template)
                    loaded = dyncfg_get_schema(string2str(df->template), rfe->result.wb);
            } else
                loaded = dyncfg_get_schema(id, rfe->result.wb);

            if (loaded) {
                rfe->result.wb->content_type = CT_APPLICATION_JSON;
                rfe->result.wb->expires = now_realtime_sec();
                rc = HTTP_RESP_OK;
                make_the_call_to_plugin = false;
            }
        }
    }

    // 4. execute the command

    if(make_the_call_to_plugin) {
        struct dyncfg_call *dc = callocz(1, sizeof(*dc));
        uuid_copy(dc->transaction.uuid, *rfe->transaction);
        dc->function = strdupz(rfe->function);
        dc->id = strdupz(id);
        dc->source = rfe->source ? strdupz(rfe->source) : NULL;
        dc->add_name = (add_name) ? strdupz(add_name) : NULL;
        dc->cmd = cmd;
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

