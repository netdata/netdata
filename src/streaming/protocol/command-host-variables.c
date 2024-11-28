// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "plugins.d/pluginsd_internals.h"

static inline void rrdpush_sender_add_host_variable_to_buffer(BUFFER *wb, const RRDVAR_ACQUIRED *rva) {
    buffer_sprintf(
        wb
        , "VARIABLE HOST %s = " NETDATA_DOUBLE_FORMAT "\n"
        , rrdvar_name(rva)
            , rrdvar2number(rva)
    );

    netdata_log_debug(D_STREAM, "RRDVAR pushed HOST VARIABLE %s = " NETDATA_DOUBLE_FORMAT, rrdvar_name(rva), rrdvar2number(rva));
}

void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva) {
    if(rrdhost_can_send_metadata_to_parent(host)) {
        BUFFER *wb = sender_start(host->sender);
        rrdpush_sender_add_host_variable_to_buffer(wb, rva);
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
        sender_commit_thread_buffer_free();
    }
}

struct custom_host_variables_callback {
    BUFFER *wb;
};

static int rrdpush_sender_thread_custom_host_variables_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar_ptr __maybe_unused, void *struct_ptr) {
    const RRDVAR_ACQUIRED *rv = (const RRDVAR_ACQUIRED *)item;
    struct custom_host_variables_callback *tmp = struct_ptr;
    BUFFER *wb = tmp->wb;

    rrdpush_sender_add_host_variable_to_buffer(wb, rv);
    return 1;
}

void rrdpush_sender_thread_send_custom_host_variables(RRDHOST *host) {
    if(rrdhost_can_send_metadata_to_parent(host)) {
        BUFFER *wb = sender_start(host->sender);
        struct custom_host_variables_callback tmp = {
            .wb = wb
        };
        int ret = rrdvar_walkthrough_read(host->rrdvars, rrdpush_sender_thread_custom_host_variables_callback, &tmp);
        (void)ret;
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
        sender_commit_thread_buffer_free();

        netdata_log_debug(D_STREAM, "RRDVAR sent %d VARIABLES", ret);
    }
}
