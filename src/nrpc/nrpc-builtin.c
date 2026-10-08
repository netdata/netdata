// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc.h"

static int nrpc_builtin_handler(struct nrpc_request *req, void *data) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    nrpc_builtin_handler_cb_t handler = data;

    int code;

    if(req->is_cancelled.cb && req->is_cancelled.cb(req->is_cancelled.data))
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    else
        code = handler(req->result.wb, req->function, req->payload, req->source);

    if(code == HTTP_RESP_CLIENT_CLOSED_REQUEST || (req->is_cancelled.cb && req->is_cancelled.cb(req->is_cancelled.data))) {
        buffer_flush(req->result.wb);
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(req->result.cb)
        req->result.cb(req->result.wb, code, req->result.data);

    return code;
}

void nrpc_method_register_builtin(const struct nrpc_builtin_desc *desc) {
    internal_fatal(!desc->handler, "NRPC: builtin registration without a handler");

    nrpc_serving_started(); // this creates a serving handle that lives for as long as netdata runs

    nrpc_method_register(&(struct nrpc_method_desc) {
        .owner = desc->owner,
        .name = desc->name,
        .help = desc->help,
        .tags = desc->tags,
        .timeout_s = desc->timeout_s,
        .priority = desc->priority,
        .version = desc->version,
        .access = desc->access,
        .sync = true,
        .source = NRPC_SOURCE_DAEMON,
        .handler = nrpc_builtin_handler,
        .handler_data = desc->handler,
    });
}
