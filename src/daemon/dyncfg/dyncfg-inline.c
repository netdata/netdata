// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg.h"

static DICTIONARY *dyncfg_nodes = NULL;

static int dyncfg_inline_callback(struct nrpc_request *req, void *data __maybe_unused) {
    char tr[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*req->call_id, tr);

    bool cancelled = req->is_cancelled.cb ? req->is_cancelled.cb(req->is_cancelled.data) : false;

    int code;
    if(cancelled)
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    else
        code = dyncfg_node_find_and_call(dyncfg_nodes, tr, req->function, req->stop_monotonic_ut, &cancelled,
                                         req->payload, req->user_access, req->source, req->result.wb);

    if(code == HTTP_RESP_CLIENT_CLOSED_REQUEST || (req->is_cancelled.cb && req->is_cancelled.cb(req->is_cancelled.data))) {
        buffer_flush(req->result.wb);
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(req->result.cb)
        req->result.cb(req->result.wb, code, req->result.data);

    return code;
}

bool dyncfg_add(const struct dyncfg_add_inline_spec *spec) {
    internal_fatal(!spec->cb, "DYNCFG: inline node addition without a callback");

    struct dyncfg_node tmp = {
        .cmds = spec->cmds,
        .type = spec->type,
        .cb = spec->cb,
        .data = spec->data,
    };
    dictionary_set(dyncfg_nodes, spec->id, &tmp, sizeof(tmp));

    if(!dyncfg_add_low_level(&(struct dyncfg_add_spec) {
        .host = spec->host,
        .id = spec->id,
        .path = spec->path,
        .status = spec->status,
        .type = spec->type,
        .source_type = spec->source_type,
        .source = spec->source,
        .cmds = spec->cmds,
        .sync = true,
        .view_access = spec->view_access,
        .edit_access = spec->edit_access,
        .handler = dyncfg_inline_callback,
    })) {
        dictionary_del(dyncfg_nodes, spec->id);
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

void dyncfg_shutdown(void) {
    if(dyncfg_nodes) {
        dictionary_destroy(dyncfg_nodes);
        dyncfg_nodes = NULL;
    }
    dyncfg_shutdown_low_level();
}
