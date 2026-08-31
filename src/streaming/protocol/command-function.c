// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"
#include "daemon/dyncfg/dyncfg.h"

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

    // Clear the changed flag FIRST, then let the renderer snapshot the
    // pending FUNCTION_DEL queue - the deleters queue before re-setting the
    // flag, so a del landing after the snapshot re-sets the flag with its
    // entry already queued and nothing is ever stranded. The clear is the
    // streaming side's half of that protocol; the renderer owns the rest.
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    // the renderer drains the pending FUNCTION_DEL queue too; it needs our
    // verdict on whether the parent can accept FUNCTION_DEL (absorbed from the
    // old stream_send_function_del gate)
    size_t configs = nrpc_catalog_render_global_functions(rrdhost_nrpc_owner(host), wb,
                                                stream_has_capability(host->sender, STREAM_CAP_FUNCTION_DEL) &&
                                                    rrdhost_can_stream_metadata_to_parent(host));

    // the synthetic "config" line is the streaming side's to append (the
    // renderer knows nothing about dyncfg) - still under the spinlock,
    // before the commit, so the wire byte-order is unchanged
    if(configs && stream_has_capability(host->sender, STREAM_CAP_DYNCFG))
        dyncfg_add_streaming(wb);

    // send it as STREAM_TRAFFIC_TYPE_METADATA, not STREAM_TRAFFIC_TYPE_FUNCTIONS
    // this is just metadata not an interactive function call
    sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    spinlock_unlock(&host->sender->global_functions_spinlock);
}
