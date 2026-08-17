// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

void stream_send_global_functions(RRDHOST *host) {
    if(!stream_has_capability(host->sender, STREAM_CAP_FUNCTIONS))
        return;

    if(unlikely(!rrdhost_can_stream_metadata_to_parent(host)))
        return;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);

    // {render + commit} must be atomic against the flag-poll call site in
    // stream_send_metrics_init() - see the global_functions_spinlock comment
    // in stream-sender-internals.h
    spinlock_lock(&host->sender->global_functions_spinlock);

    // the renderer drains the pending FUNCTION_DEL queue too; it needs our
    // verdict on whether the parent can accept FUNCTION_DEL (absorbed from the
    // old stream_send_function_del gate)
    stream_sender_send_host_functions(host, wb,
                                                stream_has_capability(host->sender, STREAM_CAP_DYNCFG),
                                                stream_has_capability(host->sender, STREAM_CAP_FUNCTION_DEL) &&
                                                    rrdhost_can_stream_metadata_to_parent(host));

    // send it as STREAM_TRAFFIC_TYPE_METADATA, not STREAM_TRAFFIC_TYPE_FUNCTIONS
    // this is just metadata not an interactive function call
    sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    spinlock_unlock(&host->sender->global_functions_spinlock);
}
