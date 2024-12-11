// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "plugins.d/pluginsd_internals.h"

static inline void stream_sender_add_host_variable_to_buffer(BUFFER *wb, const RRDVAR_ACQUIRED *rva) {
    buffer_sprintf(
        wb
        , PLUGINSD_KEYWORD_VARIABLE " HOST %s = " NETDATA_DOUBLE_FORMAT "\n"
        , rrdvar_name(rva)
        , rrdvar2number(rva)
    );

    netdata_log_debug(D_STREAM, "RRDVAR pushed HOST VARIABLE %s = " NETDATA_DOUBLE_FORMAT, rrdvar_name(rva), rrdvar2number(rva));
}

void stream_sender_send_this_host_variable_now(RRDHOST *host, const RRDVAR_ACQUIRED *rva) {
    if(rrdhost_can_stream_metadata_to_parent(host)) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        stream_sender_add_host_variable_to_buffer(wb, rva);
        sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
    }
}

struct custom_host_variables_callback {
    BUFFER *wb;
};

static int stream_sender_thread_custom_host_variables_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar_ptr __maybe_unused, void *struct_ptr) {
    const RRDVAR_ACQUIRED *rv = (const RRDVAR_ACQUIRED *)item;
    struct custom_host_variables_callback *tmp = struct_ptr;
    BUFFER *wb = tmp->wb;

    stream_sender_add_host_variable_to_buffer(wb, rv);
    return 1;
}

void stream_sender_send_custom_host_variables(RRDHOST *host) {
    if(rrdhost_can_stream_metadata_to_parent(host)) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        struct custom_host_variables_callback tmp = {
            .wb = wb
        };
        int ret = rrdvar_walkthrough_read(host->rrdvars, stream_sender_thread_custom_host_variables_callback, &tmp);
        (void)ret;
        sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

        netdata_log_debug(D_STREAM, "RRDVAR sent %d VARIABLES", ret);
    }
}
