// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

void rrdpush_send_global_functions(RRDHOST *host) {
    if(!stream_has_capability(host->sender, STREAM_CAP_FUNCTIONS))
        return;

    if(unlikely(!rrdhost_can_send_metadata_to_parent(host)))
        return;

    BUFFER *wb = sender_start(host->sender);

    rrd_global_functions_expose_rrdpush(host, wb, stream_has_capability(host->sender, STREAM_CAP_DYNCFG));

    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);

    sender_thread_buffer_free();
}
