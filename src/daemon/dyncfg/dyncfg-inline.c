// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg.h"

static DICTIONARY *dyncfg_nodes = NULL;

static int dyncfg_inline_callback(struct rrd_function_execute *rfe, void *data __maybe_unused) {
    char tr[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*rfe->transaction, tr);

    bool cancelled = rfe->is_cancelled.cb ? rfe->is_cancelled.cb(rfe->is_cancelled.data) : false;

    int code;
    if(cancelled)
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    else
        code = dyncfg_node_find_and_call(dyncfg_nodes, tr, rfe->function, rfe->stop_monotonic_ut, &cancelled,
                                         rfe->payload, rfe->user_access, rfe->source, rfe->result.wb);

    if(code == HTTP_RESP_CLIENT_CLOSED_REQUEST || (rfe->is_cancelled.cb && rfe->is_cancelled.cb(rfe->is_cancelled.data))) {
        buffer_flush(rfe->result.wb);
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(rfe->result.cb)
        rfe->result.cb(rfe->result.wb, code, rfe->result.data);

    return code;
}

bool dyncfg_add(RRDHOST *host, const char *id, const char *path,
                DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source,
                DYNCFG_CMDS cmds, HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                dyncfg_cb_t cb, void *data) {

    struct dyncfg_node tmp = {
        .cmds = cmds,
        .type = type,
        .cb = cb,
        .data = data,
    };
    dictionary_set(dyncfg_nodes, id, &tmp, sizeof(tmp));

    if(!dyncfg_add_low_level(host, id, path, status, type, source_type, source, cmds,
                             0, 0, true, view_access, edit_access,
                             dyncfg_inline_callback, NULL)) {
        dictionary_del(dyncfg_nodes, id);
        return false;
    }

    return true;
}

void dyncfg_del(RRDHOST *host, const char *id) {
    dictionary_del(dyncfg_nodes, id);
    dyncfg_del_low_level(host, id);
}

void dyncfg_status(RRDHOST *host, const char *id, DYNCFG_STATUS status) {
    dyncfg_status_low_level(host, id, status);
}

void dyncfg_init(bool load_saved) {
    dyncfg_nodes = dyncfg_nodes_dictionary_create();
    dyncfg_init_low_level(load_saved);
}
