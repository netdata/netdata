// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg.h"

static DICTIONARY *dyncfg_nodes = NULL;

static int dyncfg_inline_callback(uuid_t *transaction, BUFFER *wb, BUFFER *payload,
                                  usec_t *stop_monotonic_ut, const char *function, void *collector_data __maybe_unused,
                                  rrd_function_result_callback_t result_cb, void *result_cb_data,
                                  rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                                  rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                                  rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                                  void *register_canceller_cb_data __maybe_unused,
                                  rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                                  void *register_progresser_cb_data __maybe_unused) {
    char tr[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*transaction, tr);

    bool cancelled = is_cancelled_cb ? is_cancelled_cb(is_cancelled_cb_data) : false;

    int code;
    if(cancelled)
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    else
        code = dyncfg_node_find_and_call(dyncfg_nodes, tr, function, stop_monotonic_ut, &cancelled, payload, wb);

    if(code == HTTP_RESP_CLIENT_CLOSED_REQUEST || (is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data))) {
        buffer_flush(wb);
        code = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, code, result_cb_data);

    return code;
}

bool dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, dyncfg_cb_t cb, void *data) {
    if(dyncfg_add_low_level(host, id, path, DYNCFG_STATUS_OK, type, source_type, source, cmds, 0, 0, true, dyncfg_inline_callback, NULL)) {
        struct dyncfg_node tmp = {
            .cmds = cmds,
            .type = type,
            .cb = cb,
            .data = data,
        };
        dictionary_set(dyncfg_nodes, id, &tmp, sizeof(tmp));

        return true;
    }

    return false;
}

void dyncfg_del(RRDHOST *host, const char *id) {
    dictionary_del(dyncfg_nodes, id);
    dyncfg_del_low_level(host, id);
}

void dyncfg_init(bool load_saved) {
    dyncfg_nodes = dyncfg_nodes_dictionary_create();
    dyncfg_init_low_level(load_saved);
}
