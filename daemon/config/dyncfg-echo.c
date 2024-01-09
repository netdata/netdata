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
    DYNCFG *df;
    BUFFER *wb;
};

void dyncfg_echo_cb(BUFFER *wb __maybe_unused, int code, void *result_cb_data) {
    struct dyncfg_echo *e = result_cb_data;

    buffer_free(e->wb);
    dictionary_acquired_item_release(dyncfg_globals.nodes, e->item);

    e->wb = NULL;
    e->df = NULL;
    e->item = NULL;
    freez(e);
}

void dyncfg_echo(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id __maybe_unused, DYNCFG_CMDS cmd) {
    if(!(df->cmds & cmd))
        return;

    const char *cmd_str = dyncfg_id2cmd_one(cmd);
    if(!cmd_str) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: command given does not resolve to a known command");
        return;
    }

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;

    char buf[string_strlen(df->function) + strlen(cmd_str) + 20];
    snprintfz(buf, sizeof(buf), "%s %s", string2str(df->function), cmd_str);

    rrd_function_run(df->host, e->wb, 10, HTTP_ACCESS_ADMIN, buf, false, NULL,
                     dyncfg_echo_cb, e,
                     NULL, NULL,
                     NULL, NULL,
                     NULL, NULL);
}

static void dyncfg_echo_payload(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id __maybe_unused, const char *cmd) {
    if(!df->payload)
        return;

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;

    char buf[string_strlen(df->function) + strlen(cmd) + 20];
    snprintfz(buf, sizeof(buf), "%s %s", string2str(df->function), cmd);

    rrd_function_run(df->host, e->wb, 10, HTTP_ACCESS_ADMIN, buf, false, NULL,
                     dyncfg_echo_cb, e,
                     NULL, NULL,
                     NULL, NULL,
                     df->payload, NULL);
}

void dyncfg_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id) {
    dyncfg_echo_payload(item, df, id, "update");
}

void dyncfg_echo_add(const DICTIONARY_ITEM *template_item, DYNCFG *template_df, const char *template_id, const char *job_name) {
    char buf[strlen(job_name) + 20];
    snprintfz(buf, sizeof(buf), "add %s", job_name);
    dyncfg_echo_payload(template_item, template_df, template_id, buf);
}
