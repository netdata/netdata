// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

// ----------------------------------------------------------------------------
// echo is when we send requests to plugins without any caller
// it is used for:
// 1. the first enable/disable requests we send, and also
// 2. updates to stock or user configurations
// 3. saved dynamic jobs we need to add to templates

struct dyncfg_echo {
    const DICTIONARY_ITEM *item;
    DYNCFG *df;                     // for additions this is the job, not the template
    BUFFER *wb;
    DYNCFG_CMDS cmd;
    const char *cmd_str;
};

void dyncfg_echo_cb(BUFFER *wb __maybe_unused, int code __maybe_unused, void *result_cb_data) {
    struct dyncfg_echo *e = result_cb_data;
    DYNCFG *df = e->df;

    if(DYNCFG_RESP_SUCCESS(code)) {
        // successful response

        if(e->cmd == DYNCFG_CMD_ADD) {
            df->dyncfg.status = dyncfg_status_from_successful_response(code);
            dyncfg_update_status_on_successful_add_or_update(df, code);
        }
        else if(e->cmd == DYNCFG_CMD_UPDATE) {
            df->dyncfg.status = dyncfg_status_from_successful_response(code);
            dyncfg_update_status_on_successful_add_or_update(df, code);
        }
        else if(e->cmd == DYNCFG_CMD_DISABLE)
            df->dyncfg.status = df->current.status = DYNCFG_STATUS_DISABLED;
        else if(e->cmd == DYNCFG_CMD_ENABLE)
            df->dyncfg.status = df->current.status = dyncfg_status_from_successful_response(code);
    }
    else {
        // failed response

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "DYNCFG: received response code %d on request to id '%s', cmd: %s",
               code, dictionary_acquired_item_name(e->item), e->cmd_str);

        if(e->cmd == DYNCFG_CMD_UPDATE || e->cmd == DYNCFG_CMD_ADD)
            e->df->dyncfg.plugin_rejected = true;
    }

    buffer_free(e->wb);
    dictionary_acquired_item_release(dyncfg_globals.nodes, e->item);

    e->wb = NULL;
    e->df = NULL;
    e->item = NULL;
    freez((void *)e->cmd_str);
    e->cmd_str = NULL;
    freez(e);
}

// ----------------------------------------------------------------------------

void dyncfg_echo(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id __maybe_unused, DYNCFG_CMDS cmd) {
    RRDHOST *host = dyncfg_rrdhost(df);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot find host of configuration id '%s'", id);
        return;
    }

    if(!(df->cmds & cmd)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: attempted to echo a cmd that is not supported");
        return;
    }

    const char *cmd_str = dyncfg_id2cmd_one(cmd);
    if(!cmd_str) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: command given does not resolve to a known command");
        return;
    }

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;
    e->cmd = cmd;
    e->cmd_str = strdupz(cmd_str);

    char buf[string_strlen(df->function) + strlen(e->cmd_str) + 20];
    snprintfz(buf, sizeof(buf), "%s %s", string2str(df->function), e->cmd_str);

    rrd_function_run(
        host, e->wb, 10,
        HTTP_ACCESS_ALL, buf, false, NULL,
        dyncfg_echo_cb, e,
        NULL, NULL,
        NULL, NULL,
        NULL, string2str(df->dyncfg.source), false);
}

// ----------------------------------------------------------------------------

void dyncfg_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id) {
    RRDHOST *host = dyncfg_rrdhost(df);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot find host of configuration id '%s'", id);
        return;
    }

    if(!df->dyncfg.payload) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: requested to send an update to '%s', but there is no payload", id);
        return;
    }

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;
    e->cmd = DYNCFG_CMD_UPDATE;
    e->cmd_str = strdupz("update");

    char buf[string_strlen(df->function) + strlen(e->cmd_str) + 20];
    snprintfz(buf, sizeof(buf), "%s %s", string2str(df->function), e->cmd_str);

    rrd_function_run(
        host, e->wb, 10,
        HTTP_ACCESS_ALL, buf, false, NULL,
        dyncfg_echo_cb, e,
        NULL, NULL,
        NULL, NULL,
        df->dyncfg.payload, string2str(df->dyncfg.source), false);
}

// ----------------------------------------------------------------------------

static void dyncfg_echo_payload_add(const DICTIONARY_ITEM *item_template __maybe_unused, const DICTIONARY_ITEM *item_job, DYNCFG *df_template, DYNCFG *df_job, const char *id_template, const char *cmd) {
    RRDHOST *host = dyncfg_rrdhost(df_template);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot find host of configuration id '%s'", id_template);
        return;
    }

    if(!df_job->dyncfg.payload) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "DYNCFG: requested to send a '%s' to '%s', but there is no payload",
               cmd, id_template);
        return;
    }

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item_job);
    e->wb = buffer_create(0, NULL);
    e->df = df_job;
    e->cmd = DYNCFG_CMD_ADD;
    e->cmd_str = strdupz(cmd);

    char buf[string_strlen(df_template->function) + strlen(cmd) + 20];
    snprintfz(buf, sizeof(buf), "%s %s", string2str(df_template->function), cmd);

    rrd_function_run(
        host, e->wb, 10,
        HTTP_ACCESS_ALL, buf, false, NULL,
        dyncfg_echo_cb, e,
        NULL, NULL,
        NULL, NULL,
        df_job->dyncfg.payload, string2str(df_job->dyncfg.source), false);
}

void dyncfg_echo_add(const DICTIONARY_ITEM *item_template, const DICTIONARY_ITEM *item_job, DYNCFG *df_template, DYNCFG *df_job, const char *template_id, const char *job_name) {
    char buf[strlen(job_name) + 20];
    snprintfz(buf, sizeof(buf), "add %s", job_name);
    dyncfg_echo_payload_add(item_template, item_job, df_template, df_job, template_id, buf);
}

